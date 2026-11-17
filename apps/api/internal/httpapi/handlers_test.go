package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/featureflag/api/internal/auth"
	"github.com/featureflag/api/internal/config"
	"github.com/featureflag/api/internal/featureflag"
	"github.com/featureflag/api/internal/httpapi"
	"github.com/featureflag/api/internal/project"
	"github.com/featureflag/api/internal/sdkkey"
	"github.com/featureflag/api/internal/stream"
	"github.com/featureflag/api/internal/testsupport"
)

// loadSchema reads every migration and returns them as one DDL string.
func loadSchema(t *testing.T) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file path")
	}
	dir := filepath.Join(filepath.Dir(thisFile), "..", "..", "migrations")
	return testsupport.MigrationsDDL(t, dir)
}

// testEnv holds the pieces a handler test needs.
type testEnv struct {
	server   *httptest.Server
	pool     *pgxpool.Pool
	email    string
	password string
}

// newTestEnv starts a server backed by an isolated schema.
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	testsupport.SkipIfNoDatabase(t)
	pool, cleanup := testsupport.NewSchema(t, loadSchema(t))
	t.Cleanup(cleanup)

	ctx := context.Background()
	userID := testsupport.SeedUser(t, ctx, pool, "admin@test.local")

	const email = "admin@test.local"
	const password = "a-strong-test-password"

	// Replace the seeded user with one that has a known password.
	hash, err := auth.HashPassword(password, auth.DefaultArgon2Params())
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET password_hash = $2 WHERE id = $1`, userID, hash); err != nil {
		t.Fatalf("set password: %v", err)
	}

	cfg := &config.Config{
		SessionCookieName:    "ff_session",
		SessionAbsoluteTTL:   24 * time.Hour,
		SessionIdleTTL:       8 * time.Hour,
		CookieSecure:         false,
		CSRFSecret:           []byte("test-csrf-secret-at-least-32-bytes-long"),
		LoginRateLimitMax:    5,
		LoginRateLimitWindow: 15 * time.Minute,
		AllowedOrigins:       []string{"http://localhost:3000"},
		// A short heartbeat interval keeps the heartbeat test quick.
		SSEHeartbeatInterval: 2 * time.Second,
		SSEMaxConnection:     30 * time.Second,
		SSESubscriberQueue:   16,
		SSEMaxConnectionsKey: 10,
	}

	limiter := auth.NewRateLimiter(cfg.LoginRateLimitMax, cfg.LoginRateLimitWindow)
	authSvc := auth.NewService(pool, auth.DefaultArgon2Params(), cfg.CSRFSecret,
		cfg.SessionAbsoluteTTL, cfg.SessionIdleTTL, limiter)
	projectSvc := project.NewService(pool)
	keySvc := sdkkey.NewService(pool, nil)

	builder := config.NewBuilder(pool)
	streams := stream.NewManager(cfg.SSESubscriberQueue, slog.New(slog.NewTextHandler(io.Discard, nil)))

	// The flag service publishes through the same path as the real
	// server: rebuild the snapshot, install it, then notify subscribers.
	snapshots := config.NewStore()
	flagSvc := featureflag.NewService(pool, publisherFunc(func(envID uuid.UUID, version int64) {
		snapshot, err := builder.Build(context.Background(), envID)
		if err != nil {
			return
		}
		snapshots.Set(snapshot)
		streams.Publish(envID, snapshot.Version)
	}))
	srv := httpapi.NewServer(cfg, authSvc, projectSvc, flagSvc, keySvc, snapshots, builder, streams, slog.New(slog.NewTextHandler(io.Discard, nil)))
	// The dashboard API and the SDK API are mounted under separate
	// prefixes, exactly as they are in the server.
	mux := http.NewServeMux()
	mux.Handle("/api/v1/", http.StripPrefix("/api/v1", srv.Router()))
	mux.Handle("/sdk/v1/", http.StripPrefix("/sdk/v1", srv.SDKRouter()))
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	return &testEnv{server: ts, pool: pool, email: email, password: password}
}

// login performs a login and returns the session cookie and CSRF token.
func (e *testEnv) login(t *testing.T) (*http.Cookie, string) {
	t.Helper()

	body := bytes.NewBufferString(`{"email":"` + e.email + `","password":"` + e.password + `"}`)
	req, err := http.NewRequest(http.MethodPost, e.server.URL+"/api/v1/auth/login", body)
	if err != nil {
		t.Fatalf("build login request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.server.Client().Do(req)
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want 200", resp.StatusCode)
	}

	var payload struct {
		CSRFToken string `json:"csrf_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode login response: %v", err)
	}

	var sessionCookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "ff_session" {
			sessionCookie = c
		}
	}
	if sessionCookie == nil {
		t.Fatal("login did not set a session cookie")
	}
	return sessionCookie, payload.CSRFToken
}

// do performs an authenticated request.
func (e *testEnv) do(t *testing.T, method, path string, cookie *http.Cookie, csrfToken string, body any) *http.Response {
	t.Helper()

	var reader *bytes.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("encode body: %v", err)
		}
		reader = bytes.NewReader(encoded)
	} else {
		reader = bytes.NewReader(nil)
	}

	req, err := http.NewRequest(method, e.server.URL+path, reader)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	if csrfToken != "" {
		req.Header.Set("X-CSRF-Token", csrfToken)
	}

	resp, err := e.server.Client().Do(req)
	if err != nil {
		t.Fatalf("request %s %s: %v", method, path, err)
	}
	return resp
}

func TestProtectedRoutesRequireSession(t *testing.T) {
	e := newTestEnv(t)

	for _, path := range []string{
		"/api/v1/projects",
		"/api/v1/projects/00000000-0000-0000-0000-000000000000",
	} {
		resp := e.do(t, http.MethodGet, path, nil, "", nil)
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("GET %s status = %d, want 401", path, resp.StatusCode)
		}
	}
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	e := newTestEnv(t)

	body := bytes.NewBufferString(`{"email":"admin@test.local","password":"wrong"}`)
	req, _ := http.NewRequest(http.MethodPost, e.server.URL+"/api/v1/auth/login", body)
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.server.Client().Do(req)
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}

	// The response must not reveal whether the account exists.
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	errBody, _ := payload["error"].(map[string]any)
	if msg, _ := errBody["message"].(string); msg != "authentication required" {
		t.Errorf("message = %q, want a generic authentication message", msg)
	}
}

func TestMutationRequiresCSRFToken(t *testing.T) {
	e := newTestEnv(t)
	cookie, _ := e.login(t)

	// Without a CSRF token the request must be rejected.
	resp := e.do(t, http.MethodPost, "/api/v1/projects", cookie, "", map[string]string{
		"name": "No CSRF", "slug": "no-csrf",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", resp.StatusCode)
	}

	// With a valid token it must succeed.
	resp = e.do(t, http.MethodPost, "/api/v1/projects", cookie, "wrong-token", map[string]string{
		"name": "Bad CSRF", "slug": "bad-csrf",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status with wrong token = %d, want 403", resp.StatusCode)
	}
}

func TestCreateProjectAndEnvironments(t *testing.T) {
	e := newTestEnv(t)
	cookie, csrf := e.login(t)

	resp := e.do(t, http.MethodPost, "/api/v1/projects", cookie, csrf, map[string]string{
		"name": "My App", "slug": "my-app",
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}

	var payload struct {
		Project struct {
			ID uuid.UUID `json:"id"`
		} `json:"project"`
		Environments []struct {
			Key           string `json:"key"`
			ConfigVersion int64  `json:"config_version"`
		} `json:"environments"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(payload.Environments) != 3 {
		t.Fatalf("got %d environments, want 3", len(payload.Environments))
	}
	want := map[string]bool{"development": true, "staging": true, "production": true}
	for _, env := range payload.Environments {
		if !want[env.Key] {
			t.Errorf("unexpected environment %q", env.Key)
		}
		delete(want, env.Key)
		if env.ConfigVersion != 1 {
			t.Errorf("environment %s version = %d, want 1", env.Key, env.ConfigVersion)
		}
	}
}

func TestDuplicateSlugReturnsConflict(t *testing.T) {
	e := newTestEnv(t)
	cookie, csrf := e.login(t)

	body := map[string]string{"name": "App", "slug": "duplicate"}
	resp := e.do(t, http.MethodPost, "/api/v1/projects", cookie, csrf, body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("first create status = %d, want 201", resp.StatusCode)
	}

	resp = e.do(t, http.MethodPost, "/api/v1/projects", cookie, csrf, body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("second create status = %d, want 409", resp.StatusCode)
	}
}

func TestFlagToggleIsIdempotent(t *testing.T) {
	e := newTestEnv(t)
	cookie, csrf := e.login(t)

	// Create a project.
	resp := e.do(t, http.MethodPost, "/api/v1/projects", cookie, csrf, map[string]string{
		"name": "App", "slug": "app",
	})
	var created struct {
		Project struct {
			ID uuid.UUID `json:"id"`
		} `json:"project"`
		Environments []struct {
			ID  uuid.UUID `json:"id"`
			Key string    `json:"key"`
		} `json:"environments"`
	}
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()

	var devID uuid.UUID
	for _, env := range created.Environments {
		if env.Key == "development" {
			devID = env.ID
		}
	}

	// Create a flag.
	resp = e.do(t, http.MethodPost, "/api/v1/projects/"+created.Project.ID.String()+"/flags", cookie, csrf,
		map[string]string{"key": "new_dashboard", "description": "test"})
	var flagPayload struct {
		Flag struct {
			ID uuid.UUID `json:"id"`
		} `json:"flag"`
	}
	json.NewDecoder(resp.Body).Decode(&flagPayload)
	resp.Body.Close()

	path := "/api/v1/flags/" + flagPayload.Flag.ID.String() + "/environments/" + devID.String()

	// First toggle changes the state and bumps the version.
	resp = e.do(t, http.MethodPut, path, cookie, csrf, map[string]bool{"enabled": true})
	var first struct {
		Version int64 `json:"version"`
		Changed bool  `json:"changed"`
	}
	json.NewDecoder(resp.Body).Decode(&first)
	resp.Body.Close()

	if !first.Changed {
		t.Error("the first toggle did not report a change")
	}

	// Second toggle is a no-op: no version increment.
	resp = e.do(t, http.MethodPut, path, cookie, csrf, map[string]bool{"enabled": true})
	var second struct {
		Version int64 `json:"version"`
		Changed bool  `json:"changed"`
	}
	json.NewDecoder(resp.Body).Decode(&second)
	resp.Body.Close()

	if second.Changed {
		t.Error("a repeated toggle reported a change")
	}
	if second.Version != first.Version {
		t.Errorf("version changed on a repeated toggle: %d then %d", first.Version, second.Version)
	}
}

func TestSDKKeyIsShownOnlyOnce(t *testing.T) {
	e := newTestEnv(t)
	cookie, csrf := e.login(t)

	resp := e.do(t, http.MethodPost, "/api/v1/projects", cookie, csrf, map[string]string{
		"name": "App", "slug": "app",
	})
	var created struct {
		Project struct {
			ID uuid.UUID `json:"id"`
		} `json:"project"`
		Environments []struct {
			ID  uuid.UUID `json:"id"`
			Key string    `json:"key"`
		} `json:"environments"`
	}
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()

	var envID uuid.UUID
	for _, env := range created.Environments {
		if env.Key == "production" {
			envID = env.ID
		}
	}

	resp = e.do(t, http.MethodPost, "/api/v1/sdk-keys", cookie, csrf, map[string]any{
		"project_id":     created.Project.ID,
		"environment_id": envID,
		"name":           "backend",
		"key_type":       "server",
	})
	var keyPayload struct {
		Key string `json:"key"`
	}
	json.NewDecoder(resp.Body).Decode(&keyPayload)
	resp.Body.Close()

	if keyPayload.Key == "" {
		t.Fatal("key creation did not return a raw key")
	}
	if len(keyPayload.Key) < 40 {
		t.Errorf("raw key looks too short: %q", keyPayload.Key)
	}

	// The list must not contain the raw key.
	listPath := "/api/v1/sdk-keys?project_id=" + created.Project.ID.String() + "&environment=" + envID.String()
	resp = e.do(t, http.MethodGet, listPath, cookie, "", nil)
	var listPayload struct {
		Keys []map[string]any `json:"keys"`
	}
	json.NewDecoder(resp.Body).Decode(&listPayload)
	resp.Body.Close()

	if len(listPayload.Keys) != 1 {
		t.Fatalf("got %d keys, want 1", len(listPayload.Keys))
	}
	if _, present := listPayload.Keys[0]["key"]; present {
		t.Error("the key list contains a raw key")
	}
	if _, present := listPayload.Keys[0]["key_prefix"]; !present {
		t.Error("the key list is missing the safe prefix")
	}
}

func TestUnknownRouteReturnsJSON404(t *testing.T) {
	e := newTestEnv(t)

	resp := e.do(t, http.MethodGet, "/does-not-exist", nil, "", nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

// publisherFunc adapts a function to the featureflag.Publisher interface.
type publisherFunc func(envID uuid.UUID, version int64)

func (f publisherFunc) Publish(envID uuid.UUID, version int64) {
	f(envID, version)
}
