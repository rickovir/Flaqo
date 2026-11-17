package config

import (
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/featureflag/api/internal/domain"
)

func TestStoreSetAndGet(t *testing.T) {
	s := NewStore()
	envID := uuid.New()

	if _, ok := s.Get(envID); ok {
		t.Error("Get returned a snapshot from an empty store")
	}

	snap := &domain.Snapshot{
		EnvironmentID: envID,
		ProjectID:     uuid.New(),
		Environment:   domain.EnvironmentProduction,
		Version:       1,
		Flags:         map[string]bool{"a": true},
	}
	s.Set(snap)

	got, ok := s.Get(envID)
	if !ok {
		t.Fatal("Get did not find the snapshot that was just set")
	}
	if got.Version != snap.Version {
		t.Errorf("version = %d, want %d", got.Version, snap.Version)
	}
	if !got.Flags["a"] {
		t.Error("flag value was not preserved")
	}
}

func TestStoreSetReplacesAtomically(t *testing.T) {
	s := NewStore()
	envID := uuid.New()

	s.Set(&domain.Snapshot{EnvironmentID: envID, Version: 1, Flags: map[string]bool{"a": false}})
	s.Set(&domain.Snapshot{EnvironmentID: envID, Version: 2, Flags: map[string]bool{"a": true}})

	got, _ := s.Get(envID)
	if got.Version != 2 {
		t.Errorf("version = %d, want 2", got.Version)
	}
	if !got.Flags["a"] {
		t.Error("the newer snapshot's content was not installed")
	}
}

func TestStoreGetReturnsStableSnapshot(t *testing.T) {
	// A reader that has loaded a pointer must keep seeing a consistent
	// version and content pair even while a writer replaces it.
	s := NewStore()
	envID := uuid.New()
	s.Set(&domain.Snapshot{EnvironmentID: envID, Version: 1, Flags: map[string]bool{"a": false}})

	held, _ := s.Get(envID)
	s.Set(&domain.Snapshot{EnvironmentID: envID, Version: 2, Flags: map[string]bool{"a": true}})

	if held.Version != 1 || held.Flags["a"] {
		t.Error("a previously loaded snapshot changed underneath the reader")
	}
}

func TestStoreStale(t *testing.T) {
	s := NewStore()
	envID := uuid.New()
	s.Set(&domain.Snapshot{EnvironmentID: envID, Version: 1})

	if s.IsStale(envID) {
		t.Error("a freshly set snapshot is reported as stale")
	}

	s.MarkStale(envID)
	if !s.IsStale(envID) {
		t.Error("MarkStale did not mark the snapshot stale")
	}
	if s.StaleCount() != 1 {
		t.Errorf("StaleCount = %d, want 1", s.StaleCount())
	}

	// A successful refresh clears the stale flag.
	s.Set(&domain.Snapshot{EnvironmentID: envID, Version: 2})
	if s.IsStale(envID) {
		t.Error("Set did not clear the stale flag")
	}
	if s.StaleCount() != 0 {
		t.Errorf("StaleCount = %d, want 0", s.StaleCount())
	}
}

func TestStoreRemove(t *testing.T) {
	s := NewStore()
	envID := uuid.New()
	s.Set(&domain.Snapshot{EnvironmentID: envID, Version: 1})
	s.Remove(envID)

	if _, ok := s.Get(envID); ok {
		t.Error("Get found a snapshot after Remove")
	}
	if s.Len() != 0 {
		t.Errorf("Len = %d, want 0", s.Len())
	}
}

func TestStoreSetIgnoresNil(t *testing.T) {
	s := NewStore()
	s.Set(nil)
	if s.Len() != 0 {
		t.Errorf("Len = %d, want 0 after a nil Set", s.Len())
	}
}

// TestStoreConcurrentReadWrite exercises the shared state so that
// go test -race actually covers it: readers load snapshots while writers
// replace them.
func TestStoreConcurrentReadWrite(t *testing.T) {
	s := NewStore()
	envID := uuid.New()
	s.Set(&domain.Snapshot{EnvironmentID: envID, Version: 1, Flags: map[string]bool{"a": false}})

	const writers = 8
	const readers = 8
	const iterations = 200

	var wg sync.WaitGroup

	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 1; i <= iterations; i++ {
				s.Set(&domain.Snapshot{
					EnvironmentID: envID,
					Version:       int64(i),
					Flags:         map[string]bool{"a": i%2 == 0},
				})
			}
		}(w)
	}

	for r := 0; r < readers; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				snap, ok := s.Get(envID)
				if !ok {
					t.Error("Get lost the snapshot during concurrent writes")
					return
				}
				// A snapshot must always be internally consistent.
				if snap.Version < 1 {
					t.Errorf("snapshot version %d is invalid", snap.Version)
					return
				}
				if len(snap.Flags) != 1 {
					t.Errorf("snapshot has %d flags, want 1", len(snap.Flags))
					return
				}
			}
		}()
	}

	wg.Wait()
}
