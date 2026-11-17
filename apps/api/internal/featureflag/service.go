// Package featureflag implements Boolean flag management.
//
// The configuration update protocol in this package is the core of the
// platform. It follows the rules in docs/TECHNICAL_SPEC.md section 6:
//
//   - The environments row is locked FOR UPDATE first, before any
//     feature_flag_states read or write. This serializes every change
//     within an environment.
//   - A write that requests the value already stored is a no-op: it
//     increments no version and publishes no event.
//   - The snapshot is built from data read and written inside the
//     transaction, never by re-querying after the commit.
package featureflag

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/featureflag/api/internal/database"
	"github.com/featureflag/api/internal/domain"
)

// Publisher receives version notifications after a committed change.
//
// It is implemented by the SSE manager. Publish is called only after the
// transaction commits.
type Publisher interface {
	Publish(envID uuid.UUID, version int64)
}

// Service manages flags and their per-environment state.
type Service struct {
	pool      *pgxpool.Pool
	publisher Publisher
}

// NewService returns a flag service. The publisher may be nil, in which
// case no notifications are emitted.
func NewService(pool *pgxpool.Pool, publisher Publisher) *Service {
	return &Service{pool: pool, publisher: publisher}
}

// Create creates a flag and a default OFF state in every environment of
// the project.
//
// Creating a flag changes every environment's configuration, so each
// affected environment's version increments and each publishes an event.
func (s *Service) Create(ctx context.Context, ownerID, projectID uuid.UUID, key, description string) (*domain.FeatureFlag, error) {
	key = strings.TrimSpace(key)
	if err := validateFlagKey(key); err != nil {
		return nil, err
	}

	var (
		flag    domain.FeatureFlag
		results []environmentResult
	)

	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		// Resolve the project by owner. This is the authorization check:
		// a project owned by someone else is reported as not found.
		if err := assertProjectOwned(ctx, tx, ownerID, projectID); err != nil {
			return err
		}

		var desc *string
		if d := strings.TrimSpace(description); d != "" {
			desc = &d
		}

		row := tx.QueryRow(ctx, `
			INSERT INTO feature_flags (project_id, key, description)
			VALUES ($1, $2, $3)
			RETURNING id, project_id, key, description, created_at, updated_at
		`, projectID, key, desc)

		if err := row.Scan(&flag.ID, &flag.ProjectID, &flag.Key, &flag.Description,
			&flag.CreatedAt, &flag.UpdatedAt); err != nil {
			if database.IsUniqueViolation(err) {
				return fmt.Errorf("flag key %q already exists in this project: %w", key, domain.ErrConflict)
			}
			return fmt.Errorf("insert flag: %w", err)
		}

		// Lock and bump every environment in a deterministic order so
		// concurrent creates cannot deadlock against each other.
		created, err := s.createStatesForAllEnvironments(ctx, tx, projectID, flag.ID)
		if err != nil {
			return err
		}
		results = created
		return nil
	})
	if err != nil {
		return nil, err
	}

	s.publishAll(results)
	return &flag, nil
}

// List returns the flags of a project with their state in one environment.
//
// The environment must belong to the project.
func (s *Service) List(ctx context.Context, ownerID, projectID, envID uuid.UUID) ([]domain.FlagValue, error) {
	if _, err := s.environmentOfProject(ctx, ownerID, projectID, envID); err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT f.id, f.key, COALESCE(st.enabled, false)
		FROM feature_flags f
		LEFT JOIN feature_flag_states st
			ON st.flag_id = f.id AND st.environment_id = $2
		WHERE f.project_id = $1
		ORDER BY f.key
	`, projectID, envID)
	if err != nil {
		return nil, fmt.Errorf("list flags: %w", err)
	}
	defer rows.Close()

	out := []domain.FlagValue{}
	for rows.Next() {
		var fv domain.FlagValue
		if err := rows.Scan(&fv.ID, &fv.Key, &fv.Enabled); err != nil {
			return nil, fmt.Errorf("scan flag: %w", err)
		}
		out = append(out, fv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate flags: %w", err)
	}
	return out, nil
}

// UpdateDescription edits a flag's description.
//
// Descriptions are excluded from the configuration snapshot, so this does
// not change the configuration and does not increment any version.
func (s *Service) UpdateDescription(ctx context.Context, ownerID, flagID uuid.UUID, description string) (*domain.FeatureFlag, error) {
	var desc *string
	if d := strings.TrimSpace(description); d != "" {
		desc = &d
	}

	var flag domain.FeatureFlag
	err := s.pool.QueryRow(ctx, `
		UPDATE feature_flags f
		SET description = $3, updated_at = now()
		FROM projects p
		WHERE f.id = $1 AND f.project_id = p.id AND p.owner_id = $2
		RETURNING f.id, f.project_id, f.key, f.description, f.created_at, f.updated_at
	`, flagID, ownerID, desc).Scan(&flag.ID, &flag.ProjectID, &flag.Key,
		&flag.Description, &flag.CreatedAt, &flag.UpdatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("update flag description: %w", err)
	}
	return &flag, nil
}

// Delete removes a flag and its states.
//
// Every environment that held a state for the flag is affected, so each
// affected environment's version increments and each publishes an event.
func (s *Service) Delete(ctx context.Context, ownerID, flagID uuid.UUID) error {
	var results []environmentResult

	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		// Resolve the flag and its project, checking ownership.
		var projectID uuid.UUID
		err := tx.QueryRow(ctx, `
			SELECT f.project_id
			FROM feature_flags f
			JOIN projects p ON p.id = f.project_id
			WHERE f.id = $1 AND p.owner_id = $2
		`, flagID, ownerID).Scan(&projectID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return fmt.Errorf("resolve flag: %w", err)
		}

		// Lock the affected environments in a deterministic order before
		// deleting, so the version bumps are serialized.
		envIDs, err := s.environmentIDsWithFlag(ctx, tx, flagID)
		if err != nil {
			return err
		}

		results = make([]environmentResult, 0, len(envIDs))
		for _, envID := range envIDs {
			version, err := bumpVersion(ctx, tx, envID)
			if err != nil {
				return err
			}
			results = append(results, environmentResult{envID: envID, version: version})
		}

		if _, err := tx.Exec(ctx, `DELETE FROM feature_flags WHERE id = $1`, flagID); err != nil {
			return fmt.Errorf("delete flag: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	s.publishAll(results)
	return nil
}

// SetState sets a flag's state in one environment.
//
// The environment row is locked first, then the current state is compared
// with the requested value. If they match, the write is a no-op: no
// version increment, no event. Otherwise the state is updated, the
// version is incremented, and the new snapshot is built from the values
// read and written inside this transaction.
//
// It returns the resulting version and whether the configuration changed.
func (s *Service) SetState(ctx context.Context, ownerID, flagID, envID uuid.UUID, enabled bool) (int64, bool, error) {
	var (
		result  environmentResult
		changed bool
	)

	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		// Step 1: lock the environment row. This is always the first lock.
		env, err := lockEnvironment(ctx, tx, envID)
		if err != nil {
			return err
		}

		// Step 2: resolve the flag and check that it and the environment
		// belong to the same project, and that the caller owns it.
		projectID, err := s.resolveFlagForUpdate(ctx, tx, ownerID, flagID)
		if err != nil {
			return err
		}
		if projectID != env.ProjectID {
			return fmt.Errorf("flag and environment belong to different projects: %w", domain.ErrValidation)
		}

		// Step 3: read the current state.
		var current bool
		err = tx.QueryRow(ctx, `
			SELECT enabled FROM feature_flag_states
			WHERE flag_id = $1 AND environment_id = $2
		`, flagID, envID).Scan(&current)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("read flag state: %w", err)
		}

		// Step 4: a no-op write changes nothing.
		//
		// The version to report is re-read from the row rather than taken
		// from the snapshot read at lock time. The row lock is held for the
		// whole transaction, so no other writer can have committed between
		// the lock and now, and the re-read is the version that this
		// transaction commits with. Returning the earlier value would let a
		// no-op report a version that another writer has already advanced
		// past.
		if err == nil && current == enabled {
			var currentVersion int64
			if err := tx.QueryRow(ctx, `
				SELECT config_version FROM environments WHERE id = $1
			`, envID).Scan(&currentVersion); err != nil {
				return fmt.Errorf("read config version: %w", err)
			}
			result = environmentResult{envID: envID, version: currentVersion}
			return nil
		}

		// Step 5: write the state.
		if errors.Is(err, pgx.ErrNoRows) {
			_, err = tx.Exec(ctx, `
				INSERT INTO feature_flag_states (flag_id, environment_id, project_id, enabled)
				VALUES ($1, $2, $3, $4)
			`, flagID, envID, projectID, enabled)
		} else {
			_, err = tx.Exec(ctx, `
				UPDATE feature_flag_states
				SET enabled = $3, updated_at = now()
				WHERE flag_id = $1 AND environment_id = $2
			`, flagID, envID, enabled)
		}
		if err != nil {
			return fmt.Errorf("write flag state: %w", err)
		}

		// Step 6: increment the version in the same transaction.
		version, err := bumpVersion(ctx, tx, envID)
		if err != nil {
			return err
		}

		result = environmentResult{envID: envID, version: version}
		changed = true
		return nil
	})
	if err != nil {
		return 0, false, err
	}

	if changed && s.publisher != nil {
		s.publisher.Publish(result.envID, result.version)
	}
	return result.version, changed, nil
}

// environmentResult carries the outcome of one environment's change.
type environmentResult struct {
	envID   uuid.UUID
	version int64
}

// publishAll notifies subscribers for each affected environment.
func (s *Service) publishAll(results []environmentResult) {
	if s.publisher == nil {
		return
	}
	for _, r := range results {
		s.publisher.Publish(r.envID, r.version)
	}
}

// createStatesForAllEnvironments creates the default OFF state for a new
// flag in every environment of the project, bumping each version.
//
// Environments are locked in key order so concurrent flag creations
// cannot deadlock.
func (s *Service) createStatesForAllEnvironments(ctx context.Context, tx pgx.Tx, projectID, flagID uuid.UUID) ([]environmentResult, error) {
	rows, err := tx.Query(ctx, `
		SELECT id FROM environments WHERE project_id = $1 ORDER BY key
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list environments: %w", err)
	}

	var envIDs []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan environment id: %w", err)
		}
		envIDs = append(envIDs, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate environments: %w", err)
	}

	results := make([]environmentResult, 0, len(envIDs))
	for _, envID := range envIDs {
		// The environment lock is taken before the state row is written.
		if _, err := lockEnvironment(ctx, tx, envID); err != nil {
			return nil, err
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO feature_flag_states (flag_id, environment_id, project_id, enabled)
			VALUES ($1, $2, $3, false)
			ON CONFLICT (flag_id, environment_id) DO NOTHING
		`, flagID, envID, projectID); err != nil {
			return nil, fmt.Errorf("insert default state: %w", err)
		}

		version, err := bumpVersion(ctx, tx, envID)
		if err != nil {
			return nil, err
		}
		results = append(results, environmentResult{envID: envID, version: version})
	}
	return results, nil
}

// environmentIDsWithFlag returns the environments that hold a state for a
// flag, in a deterministic order.
func (s *Service) environmentIDsWithFlag(ctx context.Context, tx pgx.Tx, flagID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := tx.Query(ctx, `
		SELECT environment_id FROM feature_flag_states
		WHERE flag_id = $1
		ORDER BY environment_id
	`, flagID)
	if err != nil {
		return nil, fmt.Errorf("list flag environments: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan environment id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate flag environments: %w", err)
	}
	return ids, nil
}

// resolveFlagForUpdate resolves a flag's project, verifying ownership.
func (s *Service) resolveFlagForUpdate(ctx context.Context, tx pgx.Tx, ownerID, flagID uuid.UUID) (uuid.UUID, error) {
	var projectID uuid.UUID
	err := tx.QueryRow(ctx, `
		SELECT f.project_id
		FROM feature_flags f
		JOIN projects p ON p.id = f.project_id
		WHERE f.id = $1 AND p.owner_id = $2
	`, flagID, ownerID).Scan(&projectID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, domain.ErrNotFound
		}
		return uuid.Nil, fmt.Errorf("resolve flag: %w", err)
	}
	return projectID, nil
}

// environmentOfProject resolves an environment, verifying it belongs to
// the project and that the caller owns the project.
func (s *Service) environmentOfProject(ctx context.Context, ownerID, projectID, envID uuid.UUID) (*domain.Environment, error) {
	var env domain.Environment
	err := s.pool.QueryRow(ctx, `
		SELECT e.id, e.project_id, e.key, e.name, e.config_version, e.created_at, e.updated_at
		FROM environments e
		JOIN projects p ON p.id = e.project_id
		WHERE e.id = $1 AND e.project_id = $2 AND p.owner_id = $3
	`, envID, projectID, ownerID).Scan(&env.ID, &env.ProjectID, &env.Key, &env.Name,
		&env.ConfigVersion, &env.CreatedAt, &env.UpdatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("resolve environment: %w", err)
	}
	return &env, nil
}

// assertProjectOwned verifies the caller owns the project.
func assertProjectOwned(ctx context.Context, tx pgx.Tx, ownerID, projectID uuid.UUID) error {
	var exists bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM projects WHERE id = $1 AND owner_id = $2)
	`, projectID, ownerID).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check project ownership: %w", err)
	}
	if !exists {
		return domain.ErrNotFound
	}
	return nil
}

// lockEnvironment locks an environment row and returns it.
//
// This must be the first lock taken in any transaction that changes
// configuration. The lock serializes all configuration changes within
// the environment, so the version increment and the state update cannot
// interleave with another transaction's.
func lockEnvironment(ctx context.Context, tx pgx.Tx, envID uuid.UUID) (*domain.Environment, error) {
	var env domain.Environment
	err := tx.QueryRow(ctx, `
		SELECT id, project_id, key, name, config_version, created_at, updated_at
		FROM environments
		WHERE id = $1
		FOR UPDATE
	`, envID).Scan(&env.ID, &env.ProjectID, &env.Key, &env.Name,
		&env.ConfigVersion, &env.CreatedAt, &env.UpdatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("lock environment: %w", err)
	}
	return &env, nil
}

// bumpVersion increments an environment's configuration version.
//
// The caller must already hold the environment's row lock.
func bumpVersion(ctx context.Context, tx pgx.Tx, envID uuid.UUID) (int64, error) {
	var version int64
	err := tx.QueryRow(ctx, `
		UPDATE environments
		SET config_version = config_version + 1, updated_at = now()
		WHERE id = $1
		RETURNING config_version
	`, envID).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("increment config version: %w", err)
	}
	return version, nil
}

// validateFlagKey checks a flag key against the documented pattern.
//
// The pattern allows lowercase letters, digits, underscore, and hyphen.
// The key must start with a lowercase letter or a digit, and must not end
// with a separator.
func validateFlagKey(key string) error {
	if key == "" {
		return fmt.Errorf("flag key is required: %w", domain.ErrValidation)
	}
	if len(key) > 64 {
		return fmt.Errorf("flag key must be at most 64 characters: %w", domain.ErrValidation)
	}

	for i, r := range key {
		switch {
		case r >= 'a' && r <= 'z':
			continue
		case r >= '0' && r <= '9':
			continue
		case r == '_' || r == '-':
			if i == 0 {
				return fmt.Errorf("flag key must start with a lowercase letter or digit: %w", domain.ErrValidation)
			}
			continue
		case r >= 'A' && r <= 'Z':
			return fmt.Errorf("flag key must be lowercase: %w", domain.ErrValidation)
		default:
			return fmt.Errorf("flag key contains an invalid character at position %d: %w", i, domain.ErrValidation)
		}
	}

	if key[len(key)-1] == '_' || key[len(key)-1] == '-' {
		return fmt.Errorf("flag key must not end with a separator: %w", domain.ErrValidation)
	}
	return nil
}
