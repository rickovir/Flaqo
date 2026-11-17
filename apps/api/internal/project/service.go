// Package project implements project and environment management.
//
// A project always owns exactly the three default environments, created
// in the same transaction as the project.
package project

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/featureflag/api/internal/database"
	"github.com/featureflag/api/internal/domain"
)

// Service manages projects and their environments.
type Service struct {
	pool *pgxpool.Pool
}

// NewService returns a project service backed by the given pool.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// Create creates a project and its three default environments.
//
// The project and its environments are written in one transaction, so a
// project never exists without its environments.
func (s *Service) Create(ctx context.Context, ownerID uuid.UUID, name, slug string) (*domain.Project, []domain.Environment, error) {
	name = strings.TrimSpace(name)
	slug = strings.ToLower(strings.TrimSpace(slug))

	if err := validateName(name); err != nil {
		return nil, nil, err
	}
	if err := validateSlug(slug); err != nil {
		return nil, nil, err
	}

	var (
		proj domain.Project
		envs []domain.Environment
	)

	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			INSERT INTO projects (owner_id, name, slug)
			VALUES ($1, $2, $3)
			RETURNING id, owner_id, name, slug, created_at, updated_at
		`, ownerID, name, slug)

		if err := row.Scan(&proj.ID, &proj.OwnerID, &proj.Name, &proj.Slug,
			&proj.CreatedAt, &proj.UpdatedAt); err != nil {
			if database.IsUniqueViolation(err) {
				return fmt.Errorf("project slug %q already exists: %w", slug, domain.ErrConflict)
			}
			return fmt.Errorf("insert project: %w", err)
		}

		envs = make([]domain.Environment, 0, len(domain.DefaultEnvironmentKeys))
		for _, key := range domain.DefaultEnvironmentKeys {
			var env domain.Environment
			row := tx.QueryRow(ctx, `
				INSERT INTO environments (project_id, key, name, config_version)
				VALUES ($1, $2, $3, 1)
				RETURNING id, project_id, key, name, config_version, created_at, updated_at
			`, proj.ID, key, displayName(key))

			if err := row.Scan(&env.ID, &env.ProjectID, &env.Key, &env.Name,
				&env.ConfigVersion, &env.CreatedAt, &env.UpdatedAt); err != nil {
				return fmt.Errorf("insert environment %q: %w", key, err)
			}
			envs = append(envs, env)
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	return &proj, envs, nil
}

// List returns every project owned by the given administrator.
func (s *Service) List(ctx context.Context, ownerID uuid.UUID) ([]domain.Project, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, owner_id, name, slug, created_at, updated_at
		FROM projects
		WHERE owner_id = $1
		ORDER BY created_at
	`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	out := []domain.Project{}
	for rows.Next() {
		var p domain.Project
		if err := rows.Scan(&p.ID, &p.OwnerID, &p.Name, &p.Slug, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate projects: %w", err)
	}
	return out, nil
}

// Get returns a project owned by the given administrator.
//
// A project owned by someone else is reported as not found, so the
// endpoint does not reveal that the project exists.
func (s *Service) Get(ctx context.Context, ownerID, projectID uuid.UUID) (*domain.Project, error) {
	var p domain.Project
	row := s.pool.QueryRow(ctx, `
		SELECT id, owner_id, name, slug, created_at, updated_at
		FROM projects
		WHERE id = $1 AND owner_id = $2
	`, projectID, ownerID)

	if err := row.Scan(&p.ID, &p.OwnerID, &p.Name, &p.Slug, &p.CreatedAt, &p.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("get project: %w", err)
	}
	return &p, nil
}

// Update renames a project.
func (s *Service) Update(ctx context.Context, ownerID, projectID uuid.UUID, name string) (*domain.Project, error) {
	name = strings.TrimSpace(name)
	if err := validateName(name); err != nil {
		return nil, err
	}

	var p domain.Project
	row := s.pool.QueryRow(ctx, `
		UPDATE projects
		SET name = $3, updated_at = now()
		WHERE id = $1 AND owner_id = $2
		RETURNING id, owner_id, name, slug, created_at, updated_at
	`, projectID, ownerID, name)

	if err := row.Scan(&p.ID, &p.OwnerID, &p.Name, &p.Slug, &p.CreatedAt, &p.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("update project: %w", err)
	}
	return &p, nil
}

// Delete removes a project and everything under it.
//
// Deletion cascades in the database, so environments, flags, states, and
// SDK keys are removed with it.
func (s *Service) Delete(ctx context.Context, ownerID, projectID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM projects WHERE id = $1 AND owner_id = $2
	`, projectID, ownerID)
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// ListEnvironments returns the environments belonging to a project.
//
// The project is resolved by owner first, so a caller cannot list the
// environments of a project they do not own.
func (s *Service) ListEnvironments(ctx context.Context, ownerID, projectID uuid.UUID) ([]domain.Environment, error) {
	if _, err := s.Get(ctx, ownerID, projectID); err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, project_id, key, name, config_version, created_at, updated_at
		FROM environments
		WHERE project_id = $1
		ORDER BY key
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list environments: %w", err)
	}
	defer rows.Close()

	out := []domain.Environment{}
	for rows.Next() {
		var e domain.Environment
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.Key, &e.Name,
			&e.ConfigVersion, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan environment: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate environments: %w", err)
	}
	return out, nil
}

// displayName returns the human-readable label for a default environment.
func displayName(key domain.EnvironmentKey) string {
	switch key {
	case domain.EnvironmentDevelopment:
		return "Development"
	case domain.EnvironmentStaging:
		return "Staging"
	case domain.EnvironmentProduction:
		return "Production"
	default:
		return string(key)
	}
}

// validateName checks a project name.
func validateName(name string) error {
	if name == "" {
		return fmt.Errorf("name is required: %w", domain.ErrValidation)
	}
	if len(name) > 200 {
		return fmt.Errorf("name must be at most 200 characters: %w", domain.ErrValidation)
	}
	return nil
}

// validateSlug checks a project slug.
//
// The slug is a stable identifier used in URLs, so it is restricted to
// lowercase letters, digits, and hyphens.
func validateSlug(slug string) error {
	if slug == "" {
		return fmt.Errorf("slug is required: %w", domain.ErrValidation)
	}
	if len(slug) > 64 {
		return fmt.Errorf("slug must be at most 64 characters: %w", domain.ErrValidation)
	}
	for i, r := range slug {
		switch {
		case unicode.IsLetter(r) && r < unicode.MaxASCII:
			// lower case letters only
			if r >= 'a' && r <= 'z' {
				continue
			}
			return fmt.Errorf("slug must be lowercase: %w", domain.ErrValidation)
		case unicode.IsDigit(r) && r < unicode.MaxASCII:
			continue
		case r == '-':
			continue
		default:
			return fmt.Errorf("slug contains an invalid character at position %d: %w", i, domain.ErrValidation)
		}
	}
	return nil
}
