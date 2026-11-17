// Package sdkkey implements SDK credential issuance, validation, and
// revocation.
//
// A key's scope is always derived from the validated credential. The
// project and environment a caller may read are never taken from an
// untrusted request parameter.
package sdkkey

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/featureflag/api/internal/auth"
	"github.com/featureflag/api/internal/domain"
)

// Service manages SDK credentials.
type Service struct {
	pool   *pgxpool.Pool
	pepper []byte
}

// NewService returns an SDK key service. The pepper is optional; when set,
// it is applied when hashing and verifying key secrets.
func NewService(pool *pgxpool.Pool, pepper []byte) *Service {
	return &Service{pool: pool, pepper: pepper}
}

// CreatedKey is a newly issued key.
//
// Raw is populated only at creation time. It is never retrievable again,
// because only the hash is stored.
type CreatedKey struct {
	Key       domain.SDKKey
	RawKey    string
	CreatedAt time.Time
}

// Create issues a new SDK key scoped to one environment.
//
// The environment must belong to the project, and the caller must own the
// project.
func (s *Service) Create(ctx context.Context, ownerID, projectID, envID uuid.UUID, name, keyType string) (*CreatedKey, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("key name is required: %w", domain.ErrValidation)
	}
	if keyType != string(domain.SDKKeyTypeClient) && keyType != string(domain.SDKKeyTypeServer) {
		return nil, fmt.Errorf("invalid key type %q: %w", keyType, domain.ErrValidation)
	}

	// Verify the environment belongs to the project and the caller owns it.
	var ok bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM environments e
			JOIN projects p ON p.id = e.project_id
			WHERE e.id = $1 AND e.project_id = $2 AND p.owner_id = $3
		)
	`, envID, projectID, ownerID).Scan(&ok)
	if err != nil {
		return nil, fmt.Errorf("verify environment scope: %w", err)
	}
	if !ok {
		return nil, domain.ErrNotFound
	}

	generated, err := auth.GenerateSDKKey(keyType)
	if err != nil {
		return nil, fmt.Errorf("generate sdk key: %w", err)
	}

	var key domain.SDKKey
	err = s.pool.QueryRow(ctx, `
		INSERT INTO sdk_keys (project_id, environment_id, name, key_prefix, key_hash, key_type)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, project_id, environment_id, name, key_prefix, key_type, revoked_at, created_at
	`, projectID, envID, name, generated.Prefix,
		auth.HashSDKKeySecret(generated.Secret, s.pepper), keyType).
		Scan(&key.ID, &key.ProjectID, &key.EnvironmentID, &key.Name, &key.KeyPrefix,
			&key.KeyType, &key.RevokedAt, &key.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert sdk key: %w", err)
	}

	return &CreatedKey{Key: key, RawKey: generated.Full, CreatedAt: key.CreatedAt}, nil
}

// List returns the keys belonging to an environment.
//
// Revoked keys are included so the dashboard can show them.
func (s *Service) List(ctx context.Context, ownerID, projectID, envID uuid.UUID) ([]domain.SDKKey, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT k.id, k.project_id, k.environment_id, k.name, k.key_prefix, k.key_type, k.revoked_at, k.created_at
		FROM sdk_keys k
		JOIN environments e ON e.id = k.environment_id
		JOIN projects p ON p.id = k.project_id
		WHERE k.environment_id = $1 AND k.project_id = $2 AND p.owner_id = $3
		ORDER BY k.created_at
	`, envID, projectID, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list sdk keys: %w", err)
	}
	defer rows.Close()

	out := []domain.SDKKey{}
	for rows.Next() {
		var k domain.SDKKey
		if err := rows.Scan(&k.ID, &k.ProjectID, &k.EnvironmentID, &k.Name,
			&k.KeyPrefix, &k.KeyType, &k.RevokedAt, &k.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan sdk key: %w", err)
		}
		out = append(out, k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sdk keys: %w", err)
	}
	return out, nil
}

// Revoke permanently revokes a key.
//
// Revocation is permanent: there is no un-revoke. The key's active
// streams are closed by the caller through the revoker callback.
func (s *Service) Revoke(ctx context.Context, ownerID, projectID, keyID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE sdk_keys k
		SET revoked_at = now()
		FROM projects p
		WHERE k.id = $1 AND k.project_id = $2 AND p.id = k.project_id AND p.owner_id = $3
		  AND k.revoked_at IS NULL
	`, keyID, projectID, ownerID)
	if err != nil {
		return fmt.Errorf("revoke sdk key: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Either the key does not exist, is not owned by the caller, or
		// was already revoked. All are reported the same way.
		return domain.ErrNotFound
	}
	return nil
}

// Validate authenticates a presented SDK key and returns its scope.
//
// A revoked key is rejected. The returned scope is the only authority
// for what the caller may read.
func (s *Service) Validate(ctx context.Context, rawKey string) (*domain.SDKKeyScope, error) {
	keyType, _, secret, err := auth.ParseSDKKey(rawKey)
	if err != nil {
		return nil, domain.ErrUnauthorized
	}

	var scope domain.SDKKeyScope
	err = s.pool.QueryRow(ctx, `
		SELECT id, project_id, environment_id, key_type
		FROM sdk_keys
		WHERE key_hash = $1 AND revoked_at IS NULL
	`, auth.HashSDKKeySecret(secret, s.pepper)).
		Scan(&scope.KeyID, &scope.ProjectID, &scope.EnvironmentID, &scope.KeyType)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrUnauthorized
		}
		return nil, fmt.Errorf("validate sdk key: %w", err)
	}

	// The parsed type must match the stored type. A client key must not
	// be usable where a server key is expected.
	if scope.KeyType != domain.SDKKeyType(keyType) {
		return nil, domain.ErrUnauthorized
	}
	return &scope, nil
}

// IsRevoked reports whether a key has been revoked.
//
// The SSE manager uses this to revalidate a connection on its heartbeat
// tick.
func (s *Service) IsRevoked(ctx context.Context, keyID uuid.UUID) (bool, error) {
	var revoked bool
	err := s.pool.QueryRow(ctx, `
		SELECT revoked_at IS NOT NULL FROM sdk_keys WHERE id = $1
	`, keyID).Scan(&revoked)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return true, nil
		}
		return false, fmt.Errorf("check key revocation: %w", err)
	}
	return revoked, nil
}

// ActiveKeyCount returns the number of non-revoked keys for an environment.
func (s *Service) ActiveKeyCount(ctx context.Context, envID uuid.UUID) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM sdk_keys WHERE environment_id = $1 AND revoked_at IS NULL
	`, envID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count active keys: %w", err)
	}
	return n, nil
}
