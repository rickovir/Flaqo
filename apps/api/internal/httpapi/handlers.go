package httpapi

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/featureflag/api/internal/domain"
)

// ---------------------------------------------------------------------------
// Authentication
// ---------------------------------------------------------------------------

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	UserID    uuid.UUID `json:"user_id"`
	Email     string    `json:"email"`
	CSRFToken string    `json:"csrf_token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// handleLogin signs an administrator in.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, domain.ErrValidation)
		return
	}

	result, err := s.auth.Login(r.Context(), req.Email, req.Password, clientIP(r))
	if err != nil {
		writeError(w, err)
		return
	}

	http.SetCookie(w, s.sessionCookie(result.RawToken, result.Session.ExpiresAt))
	// The CSRF token is issued as a readable cookie so the browser can
	// recover it after a page reload and send it in the X-CSRF-Token
	// header. It is not a credential: it is useless without the session
	// cookie, and it grants no access on its own.
	http.SetCookie(w, s.csrfCookie(result.CSRFToken, result.Session.ExpiresAt))
	writeJSON(w, http.StatusOK, loginResponse{
		UserID:    result.Session.UserID,
		Email:     result.Session.Email,
		CSRFToken: result.CSRFToken,
		ExpiresAt: result.Session.ExpiresAt,
	})
}

// handleLogout revokes the current session.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(s.cfg.SessionCookieName)
	if err == nil && cookie.Value != "" {
		if err := s.auth.DeleteSession(r.Context(), cookie.Value); err != nil {
			writeError(w, err)
			return
		}
	}
	http.SetCookie(w, s.clearSessionCookie())
	http.SetCookie(w, s.clearCsrfCookie())
	writeJSON(w, http.StatusOK, map[string]string{"status": "signed out"})
}

// handleMe returns the current administrator.
//
// The CSRF token is returned so the dashboard can perform state-changing
// requests after a page reload. The token is also issued as a readable
// cookie at login, so the browser can recover it without a round trip.
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	session, ok := SessionFrom(r.Context())
	if !ok {
		writeError(w, domain.ErrUnauthorized)
		return
	}

	csrfToken := ""
	if cookie, err := r.Cookie(csrfCookieName); err == nil {
		csrfToken = cookie.Value
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"user_id":    session.UserID,
		"email":      session.Email,
		"csrf_token": csrfToken,
		"expires_at": session.ExpiresAt,
	})
}

// ---------------------------------------------------------------------------
// Projects
// ---------------------------------------------------------------------------

type projectRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type projectResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func toProjectResponse(p domain.Project) projectResponse {
	return projectResponse{
		ID:        p.ID,
		Name:      p.Name,
		Slug:      p.Slug,
		CreatedAt: p.CreatedAt,
		UpdatedAt: p.UpdatedAt,
	}
}

// handleListProjects lists the administrator's projects.
func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	session, err := requireSession(r)
	if err != nil {
		writeError(w, err)
		return
	}

	projects, err := s.projects.List(r.Context(), session.UserID)
	if err != nil {
		writeError(w, err)
		return
	}

	out := make([]projectResponse, 0, len(projects))
	for _, p := range projects {
		out = append(out, toProjectResponse(p))
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": out})
}

// handleCreateProject creates a project and its default environments.
func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	session, err := requireSession(r)
	if err != nil {
		writeError(w, err)
		return
	}

	var req projectRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, domain.ErrValidation)
		return
	}

	proj, envs, err := s.projects.Create(r.Context(), session.UserID, req.Name, req.Slug)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"project":      toProjectResponse(*proj),
		"environments": toEnvironmentResponses(envs),
	})
}

// handleGetProject returns one project.
func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	session, err := requireSession(r)
	if err != nil {
		writeError(w, err)
		return
	}
	projectID, err := projectIDParam(r)
	if err != nil {
		writeError(w, err)
		return
	}

	proj, err := s.projects.Get(r.Context(), session.UserID, projectID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"project": toProjectResponse(*proj)})
}

// handleUpdateProject updates a project.
func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	session, err := requireSession(r)
	if err != nil {
		writeError(w, err)
		return
	}
	projectID, err := projectIDParam(r)
	if err != nil {
		writeError(w, err)
		return
	}

	var req projectRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, domain.ErrValidation)
		return
	}

	proj, err := s.projects.Update(r.Context(), session.UserID, projectID, req.Name)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"project": toProjectResponse(*proj)})
}

// handleDeleteProject deletes a project.
func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	session, err := requireSession(r)
	if err != nil {
		writeError(w, err)
		return
	}
	projectID, err := projectIDParam(r)
	if err != nil {
		writeError(w, err)
		return
	}

	if err := s.projects.Delete(r.Context(), session.UserID, projectID); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Environments
// ---------------------------------------------------------------------------

type environmentResponse struct {
	ID            uuid.UUID `json:"id"`
	ProjectID     uuid.UUID `json:"project_id"`
	Key           string    `json:"key"`
	Name          string    `json:"name"`
	ConfigVersion int64     `json:"config_version"`
}

func toEnvironmentResponse(e domain.Environment) environmentResponse {
	return environmentResponse{
		ID:            e.ID,
		ProjectID:     e.ProjectID,
		Key:           string(e.Key),
		Name:          e.Name,
		ConfigVersion: e.ConfigVersion,
	}
}

func toEnvironmentResponses(envs []domain.Environment) []environmentResponse {
	out := make([]environmentResponse, 0, len(envs))
	for _, e := range envs {
		out = append(out, toEnvironmentResponse(e))
	}
	return out
}

// handleListEnvironments lists a project's environments.
func (s *Server) handleListEnvironments(w http.ResponseWriter, r *http.Request) {
	session, err := requireSession(r)
	if err != nil {
		writeError(w, err)
		return
	}
	projectID, err := projectIDParam(r)
	if err != nil {
		writeError(w, err)
		return
	}

	envs, err := s.projects.ListEnvironments(r.Context(), session.UserID, projectID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"environments": toEnvironmentResponses(envs)})
}

// ---------------------------------------------------------------------------
// Flags
// ---------------------------------------------------------------------------

type createFlagRequest struct {
	Key         string `json:"key"`
	Description string `json:"description"`
}

type flagResponse struct {
	ID          uuid.UUID `json:"id"`
	ProjectID   uuid.UUID `json:"project_id"`
	Key         string    `json:"key"`
	Description *string   `json:"description"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// handleListFlags lists a project's flags with their state in the
// selected environment.
func (s *Server) handleListFlags(w http.ResponseWriter, r *http.Request) {
	session, err := requireSession(r)
	if err != nil {
		writeError(w, err)
		return
	}
	projectID, err := projectIDParam(r)
	if err != nil {
		writeError(w, err)
		return
	}
	envID, err := environmentIDParam(r)
	if err != nil {
		writeError(w, err)
		return
	}

	values, err := s.flags.List(r.Context(), session.UserID, projectID, envID)
	if err != nil {
		writeError(w, err)
		return
	}

	out := make([]map[string]any, 0, len(values))
	for _, v := range values {
		out = append(out, map[string]any{
			"id":      v.ID,
			"key":     v.Key,
			"enabled": v.Enabled,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"flags": out})
}

// handleCreateFlag creates a flag and its default OFF states.
func (s *Server) handleCreateFlag(w http.ResponseWriter, r *http.Request) {
	session, err := requireSession(r)
	if err != nil {
		writeError(w, err)
		return
	}
	projectID, err := projectIDParam(r)
	if err != nil {
		writeError(w, err)
		return
	}

	var req createFlagRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, domain.ErrValidation)
		return
	}

	flag, err := s.flags.Create(r.Context(), session.UserID, projectID, req.Key, req.Description)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"flag": flagResponse{
			ID:          flag.ID,
			ProjectID:   flag.ProjectID,
			Key:         flag.Key,
			Description: flag.Description,
			CreatedAt:   flag.CreatedAt,
			UpdatedAt:   flag.UpdatedAt,
		},
	})
}

// handleUpdateFlag updates a flag's description.
func (s *Server) handleUpdateFlag(w http.ResponseWriter, r *http.Request) {
	session, err := requireSession(r)
	if err != nil {
		writeError(w, err)
		return
	}
	flagID, err := flagIDParam(r)
	if err != nil {
		writeError(w, err)
		return
	}

	var req createFlagRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, domain.ErrValidation)
		return
	}

	flag, err := s.flags.UpdateDescription(r.Context(), session.UserID, flagID, req.Description)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"flag": flagResponse{
			ID:          flag.ID,
			ProjectID:   flag.ProjectID,
			Key:         flag.Key,
			Description: flag.Description,
			CreatedAt:   flag.CreatedAt,
			UpdatedAt:   flag.UpdatedAt,
		},
	})
}

// handleDeleteFlag deletes a flag.
func (s *Server) handleDeleteFlag(w http.ResponseWriter, r *http.Request) {
	session, err := requireSession(r)
	if err != nil {
		writeError(w, err)
		return
	}
	flagID, err := flagIDParam(r)
	if err != nil {
		writeError(w, err)
		return
	}

	if err := s.flags.Delete(r.Context(), session.UserID, flagID); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type setStateRequest struct {
	Enabled bool `json:"enabled"`
}

// handleSetFlagState sets a flag's state in one environment.
//
// The endpoint is idempotent: setting the value that is already stored
// changes nothing and does not increment the configuration version.
func (s *Server) handleSetFlagState(w http.ResponseWriter, r *http.Request) {
	session, err := requireSession(r)
	if err != nil {
		writeError(w, err)
		return
	}
	flagID, err := flagIDParam(r)
	if err != nil {
		writeError(w, err)
		return
	}
	envID, err := environmentIDPathParam(r)
	if err != nil {
		writeError(w, err)
		return
	}

	var req setStateRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, domain.ErrValidation)
		return
	}

	version, changed, err := s.flags.SetState(r.Context(), session.UserID, flagID, envID, req.Enabled)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"environment_id": envID,
		"flag_id":        flagID,
		"enabled":        req.Enabled,
		"version":        version,
		"changed":        changed,
	})
}

// ---------------------------------------------------------------------------
// SDK keys
// ---------------------------------------------------------------------------

type createSDKKeyRequest struct {
	ProjectID     uuid.UUID `json:"project_id"`
	EnvironmentID uuid.UUID `json:"environment_id"`
	Name          string    `json:"name"`
	KeyType       string    `json:"key_type"`
}

type sdkKeyResponse struct {
	ID            uuid.UUID  `json:"id"`
	ProjectID     uuid.UUID  `json:"project_id"`
	EnvironmentID uuid.UUID  `json:"environment_id"`
	Name          string     `json:"name"`
	KeyType       string     `json:"key_type"`
	KeyPrefix     string     `json:"key_prefix"`
	RevokedAt     *time.Time `json:"revoked_at"`
	CreatedAt     time.Time  `json:"created_at"`
}

// handleListSDKKeys lists an environment's keys.
func (s *Server) handleListSDKKeys(w http.ResponseWriter, r *http.Request) {
	session, err := requireSession(r)
	if err != nil {
		writeError(w, err)
		return
	}

	projectID, err := uuid.Parse(r.URL.Query().Get("project_id"))
	if err != nil {
		writeError(w, domain.ErrValidation)
		return
	}
	envID, err := environmentIDParam(r)
	if err != nil {
		writeError(w, err)
		return
	}

	keys, err := s.sdkKeys.List(r.Context(), session.UserID, projectID, envID)
	if err != nil {
		writeError(w, err)
		return
	}

	out := make([]sdkKeyResponse, 0, len(keys))
	for _, k := range keys {
		out = append(out, sdkKeyResponse{
			ID:            k.ID,
			ProjectID:     k.ProjectID,
			EnvironmentID: k.EnvironmentID,
			Name:          k.Name,
			KeyType:       string(k.KeyType),
			KeyPrefix:     k.KeyPrefix,
			RevokedAt:     k.RevokedAt,
			CreatedAt:     k.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": out})
}

// handleCreateSDKKey issues a key.
//
// The raw key is returned in this response only. It is not retrievable
// afterwards, because only its hash is stored.
func (s *Server) handleCreateSDKKey(w http.ResponseWriter, r *http.Request) {
	session, err := requireSession(r)
	if err != nil {
		writeError(w, err)
		return
	}

	var req createSDKKeyRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, domain.ErrValidation)
		return
	}

	created, err := s.sdkKeys.Create(r.Context(), session.UserID, req.ProjectID, req.EnvironmentID, req.Name, req.KeyType)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":             created.Key.ID,
		"project_id":     created.Key.ProjectID,
		"environment_id": created.Key.EnvironmentID,
		"name":           created.Key.Name,
		"key_type":       string(created.Key.KeyType),
		"key_prefix":     created.Key.KeyPrefix,
		"key":            created.RawKey,
		"created_at":     created.Key.CreatedAt,
	})
}

// handleRevokeSDKKey revokes a key permanently.
func (s *Server) handleRevokeSDKKey(w http.ResponseWriter, r *http.Request) {
	session, err := requireSession(r)
	if err != nil {
		writeError(w, err)
		return
	}

	raw := chi.URLParam(r, "keyID")
	keyID, err := uuid.Parse(raw)
	if err != nil {
		writeError(w, domain.ErrNotFound)
		return
	}
	projectID, err := uuid.Parse(r.URL.Query().Get("project_id"))
	if err != nil {
		writeError(w, domain.ErrValidation)
		return
	}

	if err := s.sdkKeys.Revoke(r.Context(), session.UserID, projectID, keyID); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
