// Package domain holds the core entities and the stable error vocabulary
// shared by the service layer.
//
// It deliberately has no dependency on HTTP or SQL. Services return these
// errors and the HTTP layer maps them to status codes.
package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// EnvironmentKey identifies one of the three default environments.
type EnvironmentKey string

const (
	EnvironmentDevelopment EnvironmentKey = "development"
	EnvironmentStaging     EnvironmentKey = "staging"
	EnvironmentProduction  EnvironmentKey = "production"
)

// DefaultEnvironmentKeys lists the environments created with every project.
var DefaultEnvironmentKeys = []EnvironmentKey{
	EnvironmentDevelopment,
	EnvironmentStaging,
	EnvironmentProduction,
}

// User is a dashboard administrator.
type User struct {
	ID           uuid.UUID
	Email        string
	PasswordHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Project groups environments and flags.
type Project struct {
	ID        uuid.UUID
	OwnerID   uuid.UUID
	Name      string
	Slug      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Environment is one configuration scope within a project.
type Environment struct {
	ID            uuid.UUID
	ProjectID     uuid.UUID
	Key           EnvironmentKey
	Name          string
	ConfigVersion int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// FeatureFlag is a Boolean flag definition. It belongs to one project.
// Its state is stored separately per environment.
type FeatureFlag struct {
	ID          uuid.UUID
	ProjectID   uuid.UUID
	Key         string
	Description *string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// FeatureFlagState is the ON/OFF value of a flag in one environment.
type FeatureFlagState struct {
	ID            uuid.UUID
	FlagID        uuid.UUID
	EnvironmentID uuid.UUID
	ProjectID     uuid.UUID
	Enabled       bool
	UpdatedAt     time.Time
}

// SDKKeyType distinguishes public browser keys from confidential server keys.
type SDKKeyType string

const (
	SDKKeyTypeClient SDKKeyType = "client"
	SDKKeyTypeServer SDKKeyType = "server"
)

// SDKKey is a scoped, read-only credential for one project/environment.
type SDKKey struct {
	ID            uuid.UUID
	ProjectID     uuid.UUID
	EnvironmentID uuid.UUID
	Name          string
	KeyPrefix     string
	KeyType       SDKKeyType
	RevokedAt     *time.Time
	CreatedAt     time.Time
}

// SDKKeyScope is the access scope derived from a validated credential.
// It is the only source of project/environment scope for SDK requests.
type SDKKeyScope struct {
	KeyID         uuid.UUID
	ProjectID     uuid.UUID
	EnvironmentID uuid.UUID
	KeyType       SDKKeyType
}

// FlagValue is a flag and its Boolean value in one environment.
//
// The ID is included so a caller can address the flag directly, for
// example to change its state.
type FlagValue struct {
	ID      uuid.UUID
	Key     string
	Enabled bool
}

// Snapshot is the immutable configuration for one environment at one version.
//
// A snapshot is replaced atomically in the cache. Its Version always
// describes its own Flags: they are built together inside the
// transaction that produced the version.
type Snapshot struct {
	EnvironmentID uuid.UUID
	ProjectID     uuid.UUID
	Environment   EnvironmentKey
	Version       int64
	Flags         map[string]bool
}

// IsNewer reports whether s is a strictly newer snapshot than other.
func (s *Snapshot) IsNewer(other *Snapshot) bool {
	if other == nil {
		return true
	}
	return s.Version > other.Version
}

// Stable error vocabulary. Services return these; the HTTP layer maps them
// to status codes and stable error codes.
var (
	// ErrNotFound is returned when a requested resource does not exist or
	// is not visible to the caller.
	ErrNotFound = errors.New("not found")
	// ErrForbidden is returned when the caller is authenticated but does
	// not own the requested resource.
	ErrForbidden = errors.New("forbidden")
	// ErrUnauthorized is returned when the request carries no valid
	// credential.
	ErrUnauthorized = errors.New("unauthorized")
	// ErrConflict is returned when a write violates a uniqueness rule.
	ErrConflict = errors.New("conflict")
	// ErrValidation is returned when input fails validation.
	ErrValidation = errors.New("validation failed")
)
