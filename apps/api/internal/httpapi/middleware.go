package httpapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/featureflag/api/internal/auth"
	"github.com/featureflag/api/internal/domain"
)

// contextKey is the private key type for request-scoped values.
type contextKey int

const (
	ctxKeySession contextKey = iota
)

// SessionFrom returns the authenticated session on a request context.
func SessionFrom(ctx context.Context) (*auth.Session, bool) {
	s, ok := ctx.Value(ctxKeySession).(*auth.Session)
	return s, ok
}

// authMiddleware requires a valid dashboard session.
//
// It rejects the request before the handler runs, so no handler can
// forget to check.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(s.cfg.SessionCookieName)
		if err != nil || cookie.Value == "" {
			writeError(w, domain.ErrUnauthorized)
			return
		}

		session, err := s.auth.Validate(r.Context(), cookie.Value)
		if err != nil {
			writeError(w, domain.ErrUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), ctxKeySession, session)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// csrfMiddleware validates the CSRF token on state-changing requests.
//
// The token is bound to the session, so it is useless without a valid
// session cookie.
func (s *Server) csrfMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}

		session, ok := SessionFrom(r.Context())
		if !ok {
			writeError(w, domain.ErrUnauthorized)
			return
		}

		submitted := r.Header.Get("X-CSRF-Token")
		if !s.auth.VerifyCSRF(session, submitted) {
			writeError(w, domain.ErrForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// sessionContextMiddleware attaches the session to the context when one
// is present, without requiring it.
//
// It is used by endpoints that are readable both authenticated and
// anonymous.
func (s *Server) sessionContextMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(s.cfg.SessionCookieName)
		if err != nil || cookie.Value == "" {
			next.ServeHTTP(w, r)
			return
		}
		session, err := s.auth.Validate(r.Context(), cookie.Value)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		ctx := context.WithValue(r.Context(), ctxKeySession, session)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requireSession returns the authenticated session, or an error if the
// request was not authenticated.
func requireSession(r *http.Request) (*auth.Session, error) {
	session, ok := SessionFrom(r.Context())
	if !ok {
		return nil, domain.ErrUnauthorized
	}
	return session, nil
}

// projectIDParam reads and parses the project ID path parameter.
func projectIDParam(r *http.Request) (uuid.UUID, error) {
	raw := chi.URLParam(r, "projectID")
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, domain.ErrNotFound
	}
	return id, nil
}

// flagIDParam reads and parses the flag ID path parameter.
func flagIDParam(r *http.Request) (uuid.UUID, error) {
	raw := chi.URLParam(r, "flagID")
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, domain.ErrNotFound
	}
	return id, nil
}

// environmentIDParam reads and parses the environment ID query parameter.
//
// It is used by endpoints where the environment is a filter, such as
// listing flags.
func environmentIDParam(r *http.Request) (uuid.UUID, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("environment"))
	if raw == "" {
		return uuid.Nil, domain.ErrValidation
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, domain.ErrValidation
	}
	return id, nil
}

// environmentIDPathParam reads and parses the environment ID path
// parameter.
//
// It is used by endpoints where the environment is part of the resource
// path, such as setting a flag's state.
func environmentIDPathParam(r *http.Request) (uuid.UUID, error) {
	raw := strings.TrimSpace(chi.URLParam(r, "environmentID"))
	if raw == "" {
		return uuid.Nil, domain.ErrNotFound
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, domain.ErrNotFound
	}
	return id, nil
}

// clientIP extracts the client address for rate limiting.
func clientIP(r *http.Request) string {
	return auth.ClientIP(r.RemoteAddr, r.Header.Get("X-Forwarded-For"))
}
