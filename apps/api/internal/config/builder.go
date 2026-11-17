package config

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/featureflag/api/internal/domain"
)

// Builder builds configuration snapshots from the database.
//
// A snapshot's version and content always describe the same state. This
// is achieved by reading the environment's version and its flag values
// within a single transaction, or by building the snapshot from the
// values read and written inside the transaction that produced the
// version.
type Builder struct {
	pool *pgxpool.Pool
}

// NewBuilder returns a snapshot builder.
func NewBuilder(pool *pgxpool.Pool) *Builder {
	return &Builder{pool: pool}
}

// Build loads the snapshot for one environment.
//
// The version and the flag values are read in one transaction with the
// environment row locked, so the snapshot cannot pair a version with
// content that a concurrent writer has already changed.
func (b *Builder) Build(ctx context.Context, envID uuid.UUID) (*domain.Snapshot, error) {
	var snap domain.Snapshot

	err := b.withLockedEnvironment(ctx, envID, func(tx pgx.Tx) error {
		var envKey string
		err := tx.QueryRow(ctx, `
			SELECT key, project_id, config_version FROM environments WHERE id = $1
		`, envID).Scan(&envKey, &snap.ProjectID, &snap.Version)
		if err != nil {
			if err == pgx.ErrNoRows {
				return domain.ErrNotFound
			}
			return fmt.Errorf("read environment: %w", err)
		}
		snap.EnvironmentID = envID
		snap.Environment = domain.EnvironmentKey(envKey)

		flags, err := readFlags(ctx, tx, envID)
		if err != nil {
			return err
		}
		snap.Flags = flags
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &snap, nil
}

// BuildAll loads a snapshot for every environment in the database.
//
// It is used at startup, before the API accepts SDK traffic, so that the
// cache is populated from the source of truth.
func (b *Builder) BuildAll(ctx context.Context) ([]*domain.Snapshot, error) {
	rows, err := b.pool.Query(ctx, `SELECT id FROM environments ORDER BY id`)
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

	snapshots := make([]*domain.Snapshot, 0, len(envIDs))
	for _, envID := range envIDs {
		snap, err := b.Build(ctx, envID)
		if err != nil {
			return nil, fmt.Errorf("build snapshot for %s: %w", envID, err)
		}
		snapshots = append(snapshots, snap)
	}
	return snapshots, nil
}

// BuildFromState builds a snapshot for an environment from values already
// known to the caller.
//
// This is the path used after a committed change: the flag values come
// from the transaction that produced the version, so the snapshot cannot
// disagree with the version it reports.
//
// changed is the map of flag values that the transaction wrote. Flags
// absent from the map are read from the database.
func (b *Builder) BuildFromState(ctx context.Context, tx pgx.Tx, envID uuid.UUID, projectID uuid.UUID, version int64, envKey domain.EnvironmentKey, changed map[string]bool) (*domain.Snapshot, error) {
	flags, err := readFlags(ctx, tx, envID)
	if err != nil {
		return nil, err
	}

	// Apply the values written by the transaction. The database read above
	// may already reflect them if this is a read-after-write, but applying
	// them guarantees the snapshot matches the transaction exactly.
	for key, enabled := range changed {
		flags[key] = enabled
	}

	return &domain.Snapshot{
		EnvironmentID: envID,
		ProjectID:     projectID,
		Environment:   envKey,
		Version:       version,
		Flags:         flags,
	}, nil
}

// readFlags reads the flag values for one environment.
func readFlags(ctx context.Context, tx pgx.Tx, envID uuid.UUID) (map[string]bool, error) {
	rows, err := tx.Query(ctx, `
		SELECT f.key, COALESCE(st.enabled, false)
		FROM feature_flags f
		LEFT JOIN feature_flag_states st
			ON st.flag_id = f.id AND st.environment_id = $1
		WHERE f.project_id = (SELECT project_id FROM environments WHERE id = $1)
		ORDER BY f.key
	`, envID)
	if err != nil {
		return nil, fmt.Errorf("read flag values: %w", err)
	}
	defer rows.Close()

	flags := map[string]bool{}
	for rows.Next() {
		var (
			key     string
			enabled bool
		)
		if err := rows.Scan(&key, &enabled); err != nil {
			return nil, fmt.Errorf("scan flag value: %w", err)
		}
		flags[key] = enabled
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate flag values: %w", err)
	}
	return flags, nil
}

// withLockedEnvironment runs fn with the environment row locked.
//
// The lock is released when the transaction ends.
func (b *Builder) withLockedEnvironment(ctx context.Context, envID uuid.UUID, fn func(pgx.Tx) error) error {
	tx, err := b.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), rollbackTimeout)
		defer cancel()
		_ = tx.Rollback(rollbackCtx)
	}()

	// The lock serializes this read against concurrent configuration
	// changes in the same environment.
	if _, err := tx.Exec(ctx, `SELECT id FROM environments WHERE id = $1 FOR UPDATE`, envID); err != nil {
		return fmt.Errorf("lock environment: %w", err)
	}

	if err := fn(tx); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// rollbackTimeout bounds the rollback attempt.
const rollbackTimeout = 5 * time.Second
