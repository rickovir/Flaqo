// Package config serves the immutable configuration snapshots used for
// runtime reads.
//
// A snapshot is built inside the transaction that produced its version and
// installed atomically afterwards. Readers never take a lock: they load a
// pointer and use it. Writers replace the pointer.
//
// The store is safe for concurrent use.
package config

import (
	"sync"
	"sync/atomic"

	"github.com/google/uuid"

	"github.com/featureflag/api/internal/domain"
)

// Store holds the current snapshot for every environment.
//
// Snapshots are keyed by environment ID. The map itself is protected by a
// mutex; the snapshot contents are immutable and are published through an
// atomic pointer so readers never block on a writer.
type Store struct {
	mu        sync.RWMutex
	snapshots map[uuid.UUID]*atomic.Pointer[domain.Snapshot]
	stale     map[uuid.UUID]bool
}

// NewStore returns an empty store.
func NewStore() *Store {
	return &Store{
		snapshots: make(map[uuid.UUID]*atomic.Pointer[domain.Snapshot]),
		stale:     make(map[uuid.UUID]bool),
	}
}

// Set installs a snapshot for an environment, replacing any previous one.
//
// The snapshot is published atomically. A reader that loaded the previous
// pointer keeps using it until it finishes, so it always sees a
// self-consistent version and content pair.
func (s *Store) Set(snap *domain.Snapshot) {
	if snap == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	ptr, ok := s.snapshots[snap.EnvironmentID]
	if !ok {
		ptr = new(atomic.Pointer[domain.Snapshot])
		s.snapshots[snap.EnvironmentID] = ptr
	}
	ptr.Store(snap)
	delete(s.stale, snap.EnvironmentID)
}

// Get returns the current snapshot for an environment.
//
// The returned snapshot is immutable and safe to read for as long as the
// caller needs it, even if a concurrent write replaces it.
func (s *Store) Get(envID uuid.UUID) (*domain.Snapshot, bool) {
	s.mu.RLock()
	ptr, ok := s.snapshots[envID]
	s.mu.RUnlock()
	if !ok {
		return nil, false
	}
	snap := ptr.Load()
	if snap == nil {
		return nil, false
	}
	return snap, true
}

// MarkStale records that an environment's snapshot could not be refreshed
// after a committed write. The entry stays servable but is reported as
// degraded by Ready and Stale.
func (s *Store) MarkStale(envID uuid.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stale[envID] = true
}

// IsStale reports whether an environment's snapshot is known to be
// out of date.
func (s *Store) IsStale(envID uuid.UUID) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stale[envID]
}

// StaleCount returns the number of environments with a stale snapshot.
func (s *Store) StaleCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.stale)
}

// Remove drops an environment's snapshot. It is used when an environment
// or project is deleted.
func (s *Store) Remove(envID uuid.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.snapshots, envID)
	delete(s.stale, envID)
}

// EnvironmentIDs returns the IDs of every environment with a snapshot.
func (s *Store) EnvironmentIDs() []uuid.UUID {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]uuid.UUID, 0, len(s.snapshots))
	for id := range s.snapshots {
		out = append(out, id)
	}
	return out
}

// Len returns the number of environments with a snapshot.
func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.snapshots)
}
