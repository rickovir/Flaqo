package featureflag_test

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/featureflag/api/internal/domain"
	"github.com/featureflag/api/internal/featureflag"
	"github.com/featureflag/api/internal/project"
	"github.com/featureflag/api/internal/testsupport"
)

// loadSchema reads every migration and returns them as a single DDL
// string suitable for applying inside a test schema.
func loadSchema(t *testing.T) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file path")
	}
	// internal/featureflag -> apps/api
	dir := filepath.Join(filepath.Dir(thisFile), "..", "..", "migrations")
	return testsupport.MigrationsDDL(t, dir)
}

// setup creates an isolated schema with the full application schema and
// returns a pool plus a project owner and project.
func setup(t *testing.T) (ctx context.Context, pool *pgxpool.Pool, ownerID, projectID uuid.UUID) {
	t.Helper()

	pool, cleanup := testsupport.NewSchema(t, loadSchema(t))
	t.Cleanup(cleanup)

	ctx = context.Background()
	ownerID = testsupport.SeedUser(t, ctx, pool, "owner@test.local")

	projSvc := project.NewService(pool)
	proj, _, err := projSvc.Create(ctx, ownerID, "Test Project", "test-project")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	return ctx, pool, ownerID, proj.ID
}

func TestCreateFlagCreatesDefaultOffStatesInEveryEnvironment(t *testing.T) {
	ctx, pool, ownerID, projectID := setup(t)

	svc := featureflag.NewService(pool, nil)
	flag, err := svc.Create(ctx, ownerID, projectID, "new_dashboard", "A description")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if flag.Key != "new_dashboard" {
		t.Errorf("flag key = %q, want new_dashboard", flag.Key)
	}

	// The flag must have a default OFF state in all three environments.
	envs, err := project.NewService(pool).ListEnvironments(ctx, ownerID, projectID)
	if err != nil {
		t.Fatalf("ListEnvironments: %v", err)
	}
	if len(envs) != 3 {
		t.Fatalf("got %d environments, want 3", len(envs))
	}

	for _, env := range envs {
		values, err := svc.List(ctx, ownerID, projectID, env.ID)
		if err != nil {
			t.Fatalf("List flags in %s: %v", env.Key, err)
		}
		if len(values) != 1 {
			t.Fatalf("environment %s has %d flags, want 1", env.Key, len(values))
		}
		if values[0].Key != "new_dashboard" {
			t.Errorf("flag key = %q, want new_dashboard", values[0].Key)
		}
		if values[0].Enabled {
			t.Errorf("flag %q is ON in %s, want OFF by default", values[0].Key, env.Key)
		}
	}
}

func TestCreateFlagIncrementsEveryEnvironmentVersion(t *testing.T) {
	ctx, pool, ownerID, projectID := setup(t)

	before := versionsByEnv(t, ctx, pool, projectID)

	svc := featureflag.NewService(pool, nil)
	if _, err := svc.Create(ctx, ownerID, projectID, "version_bump", ""); err != nil {
		t.Fatalf("Create: %v", err)
	}

	after := versionsByEnv(t, ctx, pool, projectID)
	for envID, oldVersion := range before {
		if after[envID] != oldVersion+1 {
			t.Errorf("environment %s version = %d, want %d (incremented by exactly 1)",
				envID, after[envID], oldVersion+1)
		}
	}
}

func TestSetStateIsIdempotentAndDoesNotBumpVersion(t *testing.T) {
	ctx, pool, ownerID, projectID := setup(t)

	svc := featureflag.NewService(pool, nil)
	flag, err := svc.Create(ctx, ownerID, projectID, "toggle_me", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	envID := firstEnvironment(t, ctx, pool, projectID)
	versionBefore := versionOf(t, ctx, pool, envID)

	// First write changes the value.
	version, changed, err := svc.SetState(ctx, ownerID, flag.ID, envID, true)
	if err != nil {
		t.Fatalf("SetState: %v", err)
	}
	if !changed {
		t.Error("the first write did not report a change")
	}
	if version != versionBefore+1 {
		t.Errorf("version = %d, want %d", version, versionBefore+1)
	}

	// Second write requests the same value: a no-op.
	version2, changed2, err := svc.SetState(ctx, ownerID, flag.ID, envID, true)
	if err != nil {
		t.Fatalf("SetState repeat: %v", err)
	}
	if changed2 {
		t.Error("a write of the same value reported a change")
	}
	if version2 != version {
		t.Errorf("version changed on a no-op write: %d then %d", version, version2)
	}
	if got := versionOf(t, ctx, pool, envID); got != version {
		t.Errorf("stored version = %d, want %d after a no-op write", got, version)
	}
}

func TestSetStateRejectsCrossProjectEnvironment(t *testing.T) {
	ctx, pool, ownerID, projectID := setup(t)

	otherOwner := testsupport.SeedUser(t, ctx, pool, "other@test.local")
	otherProj, _, err := project.NewService(pool).Create(ctx, otherOwner, "Other", "other-project")
	if err != nil {
		t.Fatalf("create other project: %v", err)
	}

	svc := featureflag.NewService(pool, nil)
	flag, err := svc.Create(ctx, ownerID, projectID, "isolated_flag", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// An environment belonging to a different project must be rejected.
	otherEnv := firstEnvironment(t, ctx, pool, otherProj.ID)
	_, _, err = svc.SetState(ctx, ownerID, flag.ID, otherEnv, true)
	if err == nil {
		t.Fatal("SetState accepted an environment from a different project")
	}
	if !errors.Is(err, domain.ErrValidation) {
		t.Errorf("error = %v, want ErrValidation", err)
	}
}

func TestSetStateRejectsForeignOwner(t *testing.T) {
	ctx, pool, ownerID, projectID := setup(t)

	otherOwner := testsupport.SeedUser(t, ctx, pool, "intruder@test.local")

	svc := featureflag.NewService(pool, nil)
	flag, err := svc.Create(ctx, ownerID, projectID, "owned_flag", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	envID := firstEnvironment(t, ctx, pool, projectID)

	_, _, err = svc.SetState(ctx, otherOwner, flag.ID, envID, true)
	if err == nil {
		t.Fatal("SetState accepted a caller who does not own the project")
	}
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

func TestConcurrentSetStatePreservesVersionMonotonicity(t *testing.T) {
	ctx, pool, ownerID, projectID := setup(t)

	svc := featureflag.NewService(pool, nil)
	flag, err := svc.Create(ctx, ownerID, projectID, "contended", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	envID := firstEnvironment(t, ctx, pool, projectID)

	// The flag creation already bumped the environment's version, so the
	// baseline for this test is the version as it stands now.
	baseline := versionOf(t, ctx, pool, envID)

	const goroutines = 10
	var wg sync.WaitGroup
	versions := make([]int64, goroutines)
	changed := make([]bool, goroutines)
	errs := make([]error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// Alternate the value so that concurrent writes race between
			// effective changes and no-ops.
			versions[i], changed[i], errs[i] = svc.SetState(ctx, ownerID, flag.ID, envID, i%2 == 0)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: %v", i, err)
		}
	}

	// Every writer that reported an effective change must have observed a
	// distinct version. Two writers cannot commit the same version, or an
	// increment was lost.
	seen := map[int64]int{}
	effective := 0
	for i, v := range versions {
		if changed[i] {
			effective++
			seen[v]++
		}
	}
	for v, n := range seen {
		if n > 1 {
			t.Errorf("version %d was reported by %d writers that both changed the state", v, n)
		}
	}

	// The stored version must equal the baseline plus the number of
	// effective changes. No increment may be lost or double counted.
	final := versionOf(t, ctx, pool, envID)
	if want := baseline + int64(effective); final != want {
		t.Errorf("final version = %d, want %d (baseline %d plus %d effective changes)",
			final, want, baseline, effective)
	}
}

func TestConcurrentTogglesEndWithConsistentState(t *testing.T) {
	// Concurrent writes of the same value must not lose updates, and the
	// final stored value must match the final stored version.
	ctx, pool, ownerID, projectID := setup(t)

	svc := featureflag.NewService(pool, nil)
	flag, err := svc.Create(ctx, ownerID, projectID, "final_state", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	envID := firstEnvironment(t, ctx, pool, projectID)
	baseline := versionOf(t, ctx, pool, envID)

	const goroutines = 8
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// Every writer sets the same value, so at most one write is
			// effective and the rest must be no-ops.
			_, _, _ = svc.SetState(ctx, ownerID, flag.ID, envID, true)
		}(i)
	}
	wg.Wait()

	values, err := svc.List(ctx, ownerID, projectID, envID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(values) != 1 || !values[0].Enabled {
		t.Errorf("final state = %+v, want the flag ON", values)
	}

	// Only one effective change should have occurred beyond the baseline,
	// regardless of how many writers ran.
	final := versionOf(t, ctx, pool, envID)
	if want := baseline + 1; final != want {
		t.Errorf("final version = %d, want %d (baseline plus one effective write)", final, want)
	}
}

func TestDeleteFlagRemovesStates(t *testing.T) {
	ctx, pool, ownerID, projectID := setup(t)

	svc := featureflag.NewService(pool, nil)
	flag, err := svc.Create(ctx, ownerID, projectID, "doomed", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	envID := firstEnvironment(t, ctx, pool, projectID)
	if _, _, err := svc.SetState(ctx, ownerID, flag.ID, envID, true); err != nil {
		t.Fatalf("SetState: %v", err)
	}

	if err := svc.Delete(ctx, ownerID, flag.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	var count int
	testsupport.QueryRow(t, ctx, pool,
		`SELECT count(*) FROM feature_flag_states WHERE flag_id = $1`, flag.ID).Scan(&count)
	if count != 0 {
		t.Errorf("%d state rows remain after deleting the flag", count)
	}
}

func TestUpdateDescriptionDoesNotBumpVersion(t *testing.T) {
	ctx, pool, ownerID, projectID := setup(t)

	svc := featureflag.NewService(pool, nil)
	flag, err := svc.Create(ctx, ownerID, projectID, "described", "old")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	envID := firstEnvironment(t, ctx, pool, projectID)
	before := versionOf(t, ctx, pool, envID)

	updated, err := svc.UpdateDescription(ctx, ownerID, flag.ID, "new description")
	if err != nil {
		t.Fatalf("UpdateDescription: %v", err)
	}
	if updated.Description == nil || *updated.Description != "new description" {
		t.Errorf("description = %v, want %q", updated.Description, "new description")
	}

	if after := versionOf(t, ctx, pool, envID); after != before {
		t.Errorf("version changed from %d to %d after a description edit", before, after)
	}
}

func TestCreateRejectsDuplicateFlagKey(t *testing.T) {
	ctx, pool, ownerID, projectID := setup(t)

	svc := featureflag.NewService(pool, nil)
	if _, err := svc.Create(ctx, ownerID, projectID, "duplicate", ""); err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, err := svc.Create(ctx, ownerID, projectID, "duplicate", "")
	if err == nil {
		t.Fatal("Create accepted a duplicate flag key")
	}
	if !errors.Is(err, domain.ErrConflict) {
		t.Errorf("error = %v, want ErrConflict", err)
	}
}

func TestCreateRejectsInvalidFlagKey(t *testing.T) {
	ctx, pool, ownerID, projectID := setup(t)

	svc := featureflag.NewService(pool, nil)
	invalid := []string{
		"",
		"Uppercase",
		"has space",
		"_leading_underscore",
		"-leading_hyphen",
		"trailing_underscore_",
		"trailing-hyphen-",
		"emoji🙂",
		"dot.separated",
	}

	for _, key := range invalid {
		if _, err := svc.Create(ctx, ownerID, projectID, key, ""); err == nil {
			t.Errorf("Create accepted invalid flag key %q", key)
		} else if !errors.Is(err, domain.ErrValidation) {
			t.Errorf("Create(%q) error = %v, want ErrValidation", key, err)
		}
	}
}

func TestCreateAcceptsValidFlagKeys(t *testing.T) {
	ctx, pool, ownerID, projectID := setup(t)

	svc := featureflag.NewService(pool, nil)
	valid := []string{
		"a",
		"new_dashboard",
		"new-checkout",
		"feature_flag_2",
		"f",
		"9lives",
	}

	for i, key := range valid {
		if _, err := svc.Create(ctx, ownerID, projectID, key, ""); err != nil {
			t.Errorf("Create(%q) failed: %v", key, err)
			continue
		}
		_ = i
	}
}

// versionsByEnv returns each environment's current config version.
func versionsByEnv(t *testing.T, ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID) map[uuid.UUID]int64 {
	t.Helper()

	out := map[uuid.UUID]int64{}
	rows, err := pool.Query(ctx, `
		SELECT id, config_version FROM environments WHERE project_id = $1
	`, projectID)
	if err != nil {
		t.Fatalf("query versions: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var v int64
		if err := rows.Scan(&id, &v); err != nil {
			t.Fatalf("scan version: %v", err)
		}
		out[id] = v
	}
	return out
}

// firstEnvironment returns the ID of the project's first environment.
func firstEnvironment(t *testing.T, ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID) uuid.UUID {
	t.Helper()

	var id uuid.UUID
	err := pool.QueryRow(ctx, `
		SELECT id FROM environments WHERE project_id = $1 ORDER BY key LIMIT 1
	`, projectID).Scan(&id)
	if err != nil {
		t.Fatalf("find environment: %v", err)
	}
	return id
}

// versionOf returns an environment's current config version.
func versionOf(t *testing.T, ctx context.Context, pool *pgxpool.Pool, envID uuid.UUID) int64 {
	t.Helper()

	var v int64
	err := pool.QueryRow(ctx, `SELECT config_version FROM environments WHERE id = $1`, envID).Scan(&v)
	if err != nil {
		t.Fatalf("read version: %v", err)
	}
	return v
}
