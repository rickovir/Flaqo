package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/featureflag/api/internal/domain"
	"github.com/featureflag/api/internal/stream"
)

// sdkAuthMiddleware authenticates an SDK request with a scoped key.
//
// The scope comes from the validated credential. No project or
// environment is taken from the request, so a client cannot widen its
// access.
func (s *Server) sdkAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := sdkKeyFromRequest(r)
		if raw == "" {
			writeError(w, domain.ErrUnauthorized)
			return
		}

		scope, err := s.sdkKeys.Validate(r.Context(), raw)
		if err != nil {
			writeError(w, domain.ErrUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), ctxKeySDKScope, scope)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// contextKey values for request-scoped data.
const (
	ctxKeySDKScope contextKey = iota + 100
)

// sdkScopeFrom returns the validated SDK scope on a request context.
func sdkScopeFrom(r *http.Request) *domain.SDKKeyScope {
	scope, _ := r.Context().Value(ctxKeySDKScope).(*domain.SDKKeyScope)
	return scope
}

// sdkKeyFromRequest extracts the SDK key from the Authorization header.
//
// The scheme is FF. Bearer is deliberately not accepted.
func sdkKeyFromRequest(r *http.Request) string {
	header := r.Header.Get("Authorization")
	const prefix = "FF "
	if len(header) <= len(prefix) || header[:len(prefix)] != prefix {
		return ""
	}
	return header[len(prefix):]
}

// configResponse is the SDK configuration payload.
//
// It contains only flag keys, Boolean values, and the metadata the SDK
// needs. It never contains user data, session information, or the key.
type configResponse struct {
	ProjectID   uuid.UUID       `json:"project_id"`
	Environment string          `json:"environment"`
	Version     int64           `json:"version"`
	Flags       map[string]bool `json:"flags"`
}

// handleSDKConfig returns the full configuration snapshot.
//
// It reads from the in-memory store, not the database, so a config fetch
// does not query PostgreSQL.
func (s *Server) handleSDKConfig(w http.ResponseWriter, r *http.Request) {
	scope := sdkScopeFrom(r)
	if scope == nil {
		writeError(w, domain.ErrUnauthorized)
		return
	}

	snapshot, ok := s.snapshots.Get(scope.EnvironmentID)
	if !ok {
		// The cache does not have this environment. This happens right
		// after a startup that failed to load, or after the environment
		// was created since the last reload.
		s.reloadEnvironment(r, scope.EnvironmentID)
		snapshot, ok = s.snapshots.Get(scope.EnvironmentID)
		if !ok {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"error": map[string]string{
					"code":    "not_ready",
					"message": "configuration is not available",
				},
			})
			return
		}
	}

	writeJSON(w, http.StatusOK, configResponse{
		ProjectID:   snapshot.ProjectID,
		Environment: string(snapshot.Environment),
		Version:     snapshot.Version,
		Flags:       snapshot.Flags,
	})
}

// handleSDKStream streams configuration notifications.
//
// The stream carries version notifications only. The SDK fetches the
// authoritative snapshot from the config endpoint.
func (s *Server) handleSDKStream(w http.ResponseWriter, r *http.Request) {
	scope := sdkScopeFrom(r)
	if scope == nil {
		writeError(w, domain.ErrUnauthorized)
		return
	}

	// Enforce the per-credential connection limit.
	if s.streams != nil {
		if s.streams.ConnectionCount(scope.KeyID) >= s.cfg.SSEMaxConnectionsKey {
			writeJSON(w, http.StatusTooManyRequests, map[string]any{
				"error": map[string]string{
					"code":    "too_many_connections",
					"message": "connection limit reached for this credential",
				},
			})
			return
		}
	}

	sub, events, err := s.streams.Subscribe(r.Context(), scope.EnvironmentID, scope.KeyID)
	if err != nil {
		writeError(w, domain.ErrUnauthorized)
		return
	}
	defer s.streams.Unsubscribe(scope.EnvironmentID, sub.ID())

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": map[string]string{
				"code":    CodeInternal,
				"message": "streaming is not supported by this server",
			},
		})
		return
	}

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache, no-transform")
	h.Set("Connection", "keep-alive")
	// Disable proxy buffering so events are delivered immediately. This
	// header is nginx specific; the equivalent setting is applied in the
	// ingress configuration for other proxies.
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	heartbeat := time.NewTicker(s.cfg.SSEHeartbeatInterval)
	defer heartbeat.Stop()

	maxLifetime := time.NewTimer(s.cfg.SSEMaxConnection)
	defer maxLifetime.Stop()

	// The request context is cancelled when the client disconnects.
	ctx := r.Context()

	for {
		select {
		case <-ctx.Done():
			return

		case <-sub.Closed():
			// The connection was closed, for example by a key revocation.
			s.logger.Info("sse stream closed", "environment_id", scope.EnvironmentID, "key_id", scope.KeyID)
			return

		case <-maxLifetime.C:
			// The connection reached its maximum lifetime. The client
			// reconnects and fetches the latest snapshot.
			return

		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
				return
			}
			flusher.Flush()

			// Revalidate the credential on the heartbeat tick. This bounds
			// the revocation window if the immediate signal was missed.
			revoked, err := s.sdkKeys.IsRevoked(ctx, scope.KeyID)
			if err != nil {
				s.logger.Error("sse credential revalidation failed", "error", err)
			} else if revoked {
				s.logger.Info("closing sse stream for revoked key", "key_id", scope.KeyID)
				return
			}

		case event, ok := <-events:
			if !ok {
				return
			}
			if err := writeSSEEvent(w, event); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// writeSSEEvent writes one configuration notification.
//
// The event id is the environment's configuration version, so
// Last-Event-ID is interpretable as a version hint.
func writeSSEEvent(w http.ResponseWriter, event stream.Event) error {
	payload, err := json.Marshal(map[string]int64{"version": event.Version})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "id: %d\nevent: config.updated\ndata: %s\n\n", event.ID, payload)
	return err
}

// reloadEnvironment rebuilds one environment's snapshot from the database.
//
// It is used when a config fetch finds no cached snapshot, for example
// for an environment created after startup.
func (s *Server) reloadEnvironment(r *http.Request, envID uuid.UUID) {
	if s.builder == nil {
		return
	}
	snapshot, err := s.builder.Build(r.Context(), envID)
	if err != nil {
		s.logger.Error("reload environment snapshot failed", "environment_id", envID, "error", err)
		return
	}
	s.snapshots.Set(snapshot)
}

// ReloadAll rebuilds every snapshot from the database.
//
// It is called at startup before the API accepts SDK traffic, and after
// a stale snapshot is detected.
func (s *Server) ReloadAll(ctx context.Context) error {
	if s.builder == nil {
		return nil
	}
	snapshots, err := s.builder.BuildAll(ctx)
	if err != nil {
		return fmt.Errorf("build snapshots: %w", err)
	}
	for _, snapshot := range snapshots {
		s.snapshots.Set(snapshot)
	}
	s.logger.Info("configuration snapshots loaded", "count", len(snapshots))
	return nil
}
