package httpapi

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/featureflag/api/internal/auth"
	"github.com/featureflag/api/internal/config"
	"github.com/featureflag/api/internal/featureflag"
	"github.com/featureflag/api/internal/project"
	"github.com/featureflag/api/internal/sdkkey"
	"github.com/featureflag/api/internal/stream"
)

// Server holds the dependencies the HTTP handlers need.
type Server struct {
	cfg       *config.Config
	auth      *auth.Service
	projects  *project.Service
	flags     *featureflag.Service
	sdkKeys   *sdkkey.Service
	snapshots *config.Store
	builder   *config.Builder
	streams   *stream.Manager
	logger    *slog.Logger
}

// NewServer returns a Server with the given dependencies.
func NewServer(
	cfg *config.Config,
	authSvc *auth.Service,
	projects *project.Service,
	flags *featureflag.Service,
	sdkKeys *sdkkey.Service,
	snapshots *config.Store,
	builder *config.Builder,
	streams *stream.Manager,
	logger *slog.Logger,
) *Server {
	return &Server{
		cfg:       cfg,
		auth:      authSvc,
		projects:  projects,
		flags:     flags,
		sdkKeys:   sdkKeys,
		snapshots: snapshots,
		builder:   builder,
		streams:   streams,
		logger:    logger,
	}
}

// Router returns the API router.
//
// Routes under /api/v1 require a dashboard session. Routes under /sdk/v1
// require a scoped SDK key. Health endpoints are unauthenticated.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()

	r.Get("/healthz", s.handleHealthz)
	r.Get("/readyz", s.handleReadyz)

	// The router is mounted at /api/v1 by the caller, so routes here are
	// relative to that prefix.
	r.Route("/", func(r chi.Router) {
		r.Route("/auth", func(r chi.Router) {
			r.Post("/login", s.handleLogin)
			r.Post("/logout", s.handleLogout)
			r.With(s.sessionContextMiddleware).Get("/me", s.handleMe)
		})

		r.Group(func(r chi.Router) {
			r.Use(s.authMiddleware)
			r.Use(s.csrfMiddleware)

			r.Route("/projects", func(r chi.Router) {
				r.Get("/", s.handleListProjects)
				r.Post("/", s.handleCreateProject)
				r.Get("/{projectID}", s.handleGetProject)
				r.Patch("/{projectID}", s.handleUpdateProject)
				r.Delete("/{projectID}", s.handleDeleteProject)
				r.Get("/{projectID}/environments", s.handleListEnvironments)
				r.Get("/{projectID}/flags", s.handleListFlags)
				r.Post("/{projectID}/flags", s.handleCreateFlag)
			})

			r.Route("/flags", func(r chi.Router) {
				r.Patch("/{flagID}", s.handleUpdateFlag)
				r.Delete("/{flagID}", s.handleDeleteFlag)
				r.Put("/{flagID}/environments/{environmentID}", s.handleSetFlagState)
			})

			r.Route("/sdk-keys", func(r chi.Router) {
				r.Get("/", s.handleListSDKKeys)
				r.Post("/", s.handleCreateSDKKey)
				r.Delete("/{keyID}", s.handleRevokeSDKKey)
			})
		})
	})

	return r
}

// SDKRouter returns the router for the SDK endpoints.
//
// These endpoints live at /sdk/v1 and are mounted separately from the
// dashboard API at /api/v1, so an SDK credential is never presented to a
// dashboard route and vice versa.
func (s *Server) SDKRouter() http.Handler {
	r := chi.NewRouter()

	r.Group(func(r chi.Router) {
		r.Use(s.sdkAuthMiddleware)
		r.Get("/config", s.handleSDKConfig)
		r.Get("/stream", s.handleSDKStream)
	})

	return r
}

// handleHealthz reports process liveness.
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleReadyz reports readiness.
func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	response := map[string]any{
		"status":          "ready",
		"snapshots":       s.snapshots.Len(),
		"stale_snapshots": s.snapshots.StaleCount(),
	}
	if s.snapshots.StaleCount() > 0 {
		response["status"] = "degraded"
		writeJSON(w, http.StatusServiceUnavailable, response)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

// csrfCookieName is the name of the readable cookie holding the CSRF
// token.
const csrfCookieName = "ff_csrf"

// csrfCookie builds the readable CSRF token cookie.
//
// It is deliberately not HttpOnly: the dashboard's own JavaScript must
// read it to send it in the X-CSRF-Token header. It is bound to the
// session, so a third-party site that could read it still cannot use it
// without the session cookie.
func (s *Server) csrfCookie(token string, expiresAt time.Time) *http.Cookie {
	return &http.Cookie{
		Name:     csrfCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: false,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	}
}

// sessionCookie builds the session cookie.
func (s *Server) sessionCookie(rawToken string, expiresAt time.Time) *http.Cookie {
	return &http.Cookie{
		Name:     s.cfg.SessionCookieName,
		Value:    rawToken,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	}
}

// clearCsrfCookie builds a cookie that removes the CSRF token cookie.
func (s *Server) clearCsrfCookie() *http.Cookie {
	return &http.Cookie{
		Name:     csrfCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: false,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	}
}

// clearSessionCookie builds a cookie that removes the session cookie.
func (s *Server) clearSessionCookie() *http.Cookie {
	return &http.Cookie{
		Name:     s.cfg.SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	}
}
