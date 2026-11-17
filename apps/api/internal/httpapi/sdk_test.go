package httpapi_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// sseEvent is a parsed SSE frame.
type sseEvent struct {
	ID      string
	Event   string
	Data    string
	Comment string
}

// readSSEEvent reads one SSE frame from the reader.
//
// It handles the SSE wire format directly: comment lines starting with
// a colon, fields, and a blank line terminator.
func readSSEEvent(r *bufio.Reader) (sseEvent, error) {
	var ev sseEvent

	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return ev, err
		}
		line = strings.TrimRight(line, "\r\n")

		if line == "" {
			// Blank line ends the frame.
			if ev.Event != "" || ev.Comment != "" {
				return ev, nil
			}
			continue
		}

		if strings.HasPrefix(line, ":") {
			ev.Comment = strings.TrimSpace(line[1:])
			continue
		}

		field, value, found := strings.Cut(line, ":")
		if !found {
			field, value = line, ""
		}
		value = strings.TrimPrefix(value, " ")

		switch field {
		case "id":
			ev.ID = value
		case "event":
			ev.Event = value
		case "data":
			ev.Data = value
		}
	}
}

// createSDKKey issues a key and returns the raw key and the environment
// it is scoped to.
func (e *testEnv) createSDKKey(t *testing.T, cookie *http.Cookie, csrfToken string, projectID, envID uuid.UUID) string {
	t.Helper()

	resp := e.do(t, http.MethodPost, "/api/v1/sdk-keys", cookie, csrfToken, map[string]any{
		"project_id":     projectID,
		"environment_id": envID,
		"name":           "test",
		"key_type":       "server",
	})
	defer resp.Body.Close()

	var payload struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode key: %v", err)
	}
	if payload.Key == "" {
		t.Fatal("no key returned")
	}
	return payload.Key
}

// setupProject creates a project and returns its ID and environment IDs.
func (e *testEnv) setupProject(t *testing.T, cookie *http.Cookie, csrfToken string) (uuid.UUID, map[string]uuid.UUID) {
	t.Helper()

	resp := e.do(t, http.MethodPost, "/api/v1/projects", cookie, csrfToken, map[string]string{
		"name": "App", "slug": "sdk-app-" + uuid.NewString()[:8],
	})
	defer resp.Body.Close()

	var created struct {
		Project struct {
			ID uuid.UUID `json:"id"`
		} `json:"project"`
		Environments []struct {
			ID  uuid.UUID `json:"id"`
			Key string    `json:"key"`
		} `json:"environments"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode project: %v", err)
	}

	envs := map[string]uuid.UUID{}
	for _, env := range created.Environments {
		envs[env.Key] = env.ID
	}
	return created.Project.ID, envs
}

// sdkGet performs an SDK request with the FF authorization scheme.
func (e *testEnv) sdkGet(t *testing.T, path, rawKey string) *http.Response {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, e.server.URL+path, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Authorization", "FF "+rawKey)

	resp, err := e.server.Client().Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	return resp
}

func TestSDKConfigRequiresKey(t *testing.T) {
	e := newTestEnv(t)

	req, _ := http.NewRequest(http.MethodGet, e.server.URL+"/sdk/v1/config", nil)
	resp, err := e.server.Client().Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

func TestSDKConfigRejectsBearerScheme(t *testing.T) {
	e := newTestEnv(t)
	cookie, csrf := e.login(t)
	projectID, envs := e.setupProject(t, cookie, csrf)
	rawKey := e.createSDKKey(t, cookie, csrf, projectID, envs["production"])

	// Bearer must not be accepted as an alias for the FF scheme.
	req, _ := http.NewRequest(http.MethodGet, e.server.URL+"/sdk/v1/config", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)

	resp, err := e.server.Client().Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status with Bearer = %d, want 401", resp.StatusCode)
	}
}

func TestSDKConfigRejectsUnknownKey(t *testing.T) {
	e := newTestEnv(t)

	resp := e.sdkGet(t, "/sdk/v1/config", "ff_server_aaaaaaaaaaaa_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

func TestSDKConfigReturnsSnapshot(t *testing.T) {
	e := newTestEnv(t)
	cookie, csrf := e.login(t)
	projectID, envs := e.setupProject(t, cookie, csrf)
	rawKey := e.createSDKKey(t, cookie, csrf, projectID, envs["production"])

	// Create a flag so the snapshot has content.
	resp := e.do(t, http.MethodPost, "/api/v1/projects/"+projectID.String()+"/flags", cookie, csrf,
		map[string]string{"key": "sdk_flag"})
	resp.Body.Close()

	// The snapshot cache is populated at startup, so a newly created
	// project is not in it. Fetching it triggers a reload.
	resp = e.sdkGet(t, "/sdk/v1/config", rawKey)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var payload struct {
		ProjectID   uuid.UUID       `json:"project_id"`
		Environment string          `json:"environment"`
		Version     int64           `json:"version"`
		Flags       map[string]bool `json:"flags"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if payload.ProjectID != projectID {
		t.Errorf("project_id = %s, want %s", payload.ProjectID, projectID)
	}
	if payload.Environment != "production" {
		t.Errorf("environment = %q, want production", payload.Environment)
	}
	if payload.Version < 1 {
		t.Errorf("version = %d, want at least 1", payload.Version)
	}
	if value, ok := payload.Flags["sdk_flag"]; !ok || value {
		t.Errorf("flag sdk_flag = %v (present %v), want false", value, ok)
	}
}

func TestSDKConfigScopeIsLimitedToKeyEnvironment(t *testing.T) {
	e := newTestEnv(t)
	cookie, csrf := e.login(t)
	projectID, envs := e.setupProject(t, cookie, csrf)

	// A key scoped to production must not be able to read development.
	prodKey := e.createSDKKey(t, cookie, csrf, projectID, envs["production"])

	resp := e.sdkGet(t, "/sdk/v1/config", prodKey)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var payload struct {
		Environment string `json:"environment"`
	}
	json.NewDecoder(resp.Body).Decode(&payload)

	if payload.Environment != "production" {
		t.Errorf("environment = %q, want production", payload.Environment)
	}
}

func TestRevokedKeyCannotFetchConfig(t *testing.T) {
	e := newTestEnv(t)
	cookie, csrf := e.login(t)
	projectID, envs := e.setupProject(t, cookie, csrf)
	rawKey := e.createSDKKey(t, cookie, csrf, projectID, envs["production"])

	// It works before revocation.
	resp := e.sdkGet(t, "/sdk/v1/config", rawKey)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status before revocation = %d, want 200", resp.StatusCode)
	}

	// Revoke it.
	listPath := "/api/v1/sdk-keys?project_id=" + projectID.String() + "&environment=" + envs["production"].String()
	resp = e.do(t, http.MethodGet, listPath, cookie, "", nil)
	var listed struct {
		Keys []struct {
			ID uuid.UUID `json:"id"`
		} `json:"keys"`
	}
	json.NewDecoder(resp.Body).Decode(&listed)
	resp.Body.Close()

	if len(listed.Keys) != 1 {
		t.Fatalf("got %d keys, want 1", len(listed.Keys))
	}

	resp = e.do(t, http.MethodDelete,
		"/api/v1/sdk-keys/"+listed.Keys[0].ID.String()+"?project_id="+projectID.String(),
		cookie, csrf, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("revoke status = %d, want 204", resp.StatusCode)
	}

	// Now it must be rejected.
	resp = e.sdkGet(t, "/sdk/v1/config", rawKey)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status after revocation = %d, want 401", resp.StatusCode)
	}
}

func TestSDKStreamSendsVersionEvent(t *testing.T) {
	e := newTestEnv(t)
	cookie, csrf := e.login(t)
	projectID, envs := e.setupProject(t, cookie, csrf)
	rawKey := e.createSDKKey(t, cookie, csrf, projectID, envs["production"])

	// Open the stream.
	req, _ := http.NewRequest(http.MethodGet, e.server.URL+"/sdk/v1/stream", nil)
	req.Header.Set("Authorization", "FF "+rawKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stream status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("content type = %q, want text/event-stream", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "no-cache") {
		t.Errorf("cache control = %q, want no-cache", cc)
	}

	reader := bufio.NewReader(resp.Body)

	// Create the flag first, then toggle it. Flag creation already
	// publishes to every environment, so the stream receives events for
	// both operations.
	createResp := e.do(t, http.MethodPost, "/api/v1/projects/"+projectID.String()+"/flags",
		cookie, csrf, map[string]string{"key": "stream_flag"})
	var created struct {
		Flag struct {
			ID uuid.UUID `json:"id"`
		} `json:"flag"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode created flag: %v", err)
	}
	createResp.Body.Close()
	if created.Flag.ID == uuid.Nil {
		t.Fatal("created flag has no id")
	}

	// Toggle in the background so the reader is already waiting. The
	// goroutine is joined before the test returns, so it cannot outlive
	// the test server.
	toggled := make(chan struct{})
	go func() {
		defer close(toggled)
		time.Sleep(300 * time.Millisecond)
		toggleResp := e.do(t, http.MethodPut,
			"/api/v1/flags/"+created.Flag.ID.String()+"/environments/"+envs["production"].String(),
			cookie, csrf, map[string]bool{"enabled": true})
		toggleResp.Body.Close()
	}()
	defer func() { <-toggled }()

	// Read events until a config.updated arrives or the deadline passes.
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		ev, err := readSSEEvent(reader)
		if err != nil {
			t.Fatalf("read event: %v", err)
		}
		if ev.Event == "config.updated" {
			if ev.ID == "" {
				t.Error("event has no id")
			}
			var data struct {
				Version int64 `json:"version"`
			}
			if err := json.Unmarshal([]byte(ev.Data), &data); err != nil {
				t.Errorf("event data %q is not JSON: %v", ev.Data, err)
			}
			if data.Version < 1 {
				t.Errorf("event version = %d, want at least 1", data.Version)
			}
			if ev.ID != fmt.Sprintf("%d", data.Version) {
				t.Errorf("event id %q does not match the version %d", ev.ID, data.Version)
			}
			return
		}
	}
	t.Error("no config.updated event received")
}

func TestSDKStreamSendsHeartbeat(t *testing.T) {
	e := newTestEnv(t)
	cookie, csrf := e.login(t)
	projectID, envs := e.setupProject(t, cookie, csrf)
	rawKey := e.createSDKKey(t, cookie, csrf, projectID, envs["production"])

	req, _ := http.NewRequest(http.MethodGet, e.server.URL+"/sdk/v1/stream", nil)
	req.Header.Set("Authorization", "FF "+rawKey)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)

	// The heartbeat interval is 15 seconds by default, so this waits for
	// one. A short interval is configured for the test server.
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		ev, err := readSSEEvent(reader)
		if err != nil {
			t.Fatalf("read event: %v", err)
		}
		if ev.Comment == "heartbeat" {
			return
		}
	}
	t.Error("no heartbeat received")
}
