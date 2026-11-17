// Command server runs the feature flag API.
//
// It wires together configuration, the database pool, the configuration
// snapshot cache, the HTTP router, and graceful shutdown.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/google/uuid"

	"github.com/featureflag/api/internal/auth"
	"github.com/featureflag/api/internal/config"
	"github.com/featureflag/api/internal/database"
	"github.com/featureflag/api/internal/featureflag"
	"github.com/featureflag/api/internal/httpapi"
	"github.com/featureflag/api/internal/middleware"
	"github.com/featureflag/api/internal/project"
	"github.com/featureflag/api/internal/sdkkey"
	"github.com/featureflag/api/internal/stream"
)

func main() {
	// The health check runs as a separate entry point in the same binary,
	// so the container image needs no extra tooling.
	if len(os.Args) > 1 && os.Args[1] == "-healthcheck" {
		if err := runHealthcheck(); err != nil {
			fmt.Fprintf(os.Stderr, "unhealthy: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	logger.Info("config loaded", "config", cfg.String())

	// Signal-aware context: a Ctrl-C or SIGTERM starts graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	connectCtx, cancelConnect := context.WithTimeout(ctx, cfg.DatabaseConnectTimeout)
	defer cancelConnect()

	pool, err := database.Connect(connectCtx, cfg)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer pool.Close()
	logger.Info("database connected")

	snapshots := config.NewStore()

	// Services.
	limiter := auth.NewRateLimiter(cfg.LoginRateLimitMax, cfg.LoginRateLimitWindow)
	authSvc := auth.NewService(pool.Pool, auth.Argon2Params{
		Time:       cfg.Argon2Time,
		MemoryKiB:  cfg.Argon2MemoryKiB,
		Threads:    cfg.Argon2Threads,
		KeyLength:  cfg.Argon2KeyLength,
		SaltLength: cfg.Argon2SaltLength,
	}, cfg.CSRFSecret, cfg.SessionAbsoluteTTL, cfg.SessionIdleTTL, limiter)
	projectSvc := project.NewService(pool.Pool)
	sdkKeySvc := sdkkey.NewService(pool.Pool, cfg.SDKKeyPepper)

	// The publisher installs the new snapshot and then notifies
	// subscribers. Both happen only after the transaction committed.
	builder := config.NewBuilder(pool.Pool)
	streams := stream.NewManager(cfg.SSESubscriberQueue, logger)
	flagSvc := featureflag.NewService(pool.Pool, publisherFunc(func(envID uuid.UUID, version int64) {
		publishUpdate(ctx, builder, snapshots, streams, logger, envID)
	}))

	api := httpapi.NewServer(cfg, authSvc, projectSvc, flagSvc, sdkKeySvc, snapshots, builder, streams, logger)

	// Load the configuration snapshots before the API accepts traffic. The
	// SDK config endpoint reads from memory, so an empty cache would serve
	// no configuration.
	loadCtx, cancelLoad := context.WithTimeout(ctx, 30*time.Second)
	if err := api.ReloadAll(loadCtx); err != nil {
		cancelLoad()
		return fmt.Errorf("load configuration snapshots: %w", err)
	}
	cancelLoad()

	router := buildRouter(cfg, pool, snapshots, api, logger)

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       cfg.ReadTimeout,
		// WriteTimeout is disabled because SSE connections are long-lived.
		// Per-request deadlines are applied instead.
		WriteTimeout: 0,
		IdleTimeout:  cfg.IdleTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("server listening", "addr", cfg.ListenAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("serve: %w", err)
		}
		return nil
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	// Close the SSE streams first so clients reconnect to the replacement
	// instance rather than holding a half-open connection.
	streams.Close()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	logger.Info("server stopped")
	return nil
}

// publisherFunc adapts a function to the featureflag.Publisher interface.
type publisherFunc func(envID uuid.UUID, version int64)

func (f publisherFunc) Publish(envID uuid.UUID, version int64) {
	f(envID, version)
}

// publishUpdate rebuilds an environment's snapshot and notifies its
// subscribers.
//
// It runs after the transaction that produced the change has committed,
// so the snapshot it installs reflects committed data. The snapshot is
// rebuilt from the database under the environment lock, which guarantees
// that its version matches its content.
func publishUpdate(ctx context.Context, builder *config.Builder, snapshots *config.Store, streams *stream.Manager, logger *slog.Logger, envID uuid.UUID) {
	snapshot, err := builder.Build(ctx, envID)
	if err != nil {
		logger.Error("rebuild snapshot after commit failed", "environment_id", envID, "error", err)
		// The cache is now known to be out of date. Mark it stale so
		// readiness reports the degradation, and do not publish a version
		// the cache cannot serve.
		snapshots.MarkStale(envID)
		return
	}

	// Install the snapshot before publishing, so a client that reacts to
	// the event by fetching immediately receives the new configuration.
	snapshots.Set(snapshot)
	streams.Publish(envID, snapshot.Version)
}

// buildRouter assembles the HTTP handler.
func buildRouter(cfg *config.Config, pool *database.Pool, snapshots *config.Store, api *httpapi.Server, logger *slog.Logger) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestIDMiddleware)
	r.Use(middleware.RecoverMiddleware(logger))
	r.Use(middleware.LoggerMiddleware(logger))
	r.Use(middleware.CORSMiddleware(cfg.AllowedOrigins))

	r.Get("/healthz", func(w http.ResponseWriter, req *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Get("/readyz", func(w http.ResponseWriter, req *http.Request) {
		ctx, cancel := context.WithTimeout(req.Context(), cfg.DatabaseConnectTimeout)
		defer cancel()

		if err := pool.Ping(ctx); err != nil {
			logger.Error("readiness check failed", "error", err)
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"status": "not_ready",
				"reason": "database unavailable",
			})
			return
		}

		// The snapshot cache is populated during startup. An empty cache with
		// environments present in the database means the load failed.
		response := map[string]any{
			"status":          "ready",
			"database":        "ok",
			"snapshots":       snapshots.Len(),
			"stale_snapshots": snapshots.StaleCount(),
		}
		if snapshots.StaleCount() > 0 {
			response["status"] = "degraded"
			writeJSON(w, http.StatusServiceUnavailable, response)
			return
		}
		writeJSON(w, http.StatusOK, response)
	})

	r.Mount("/api/v1", api.Router())
	// The SDK endpoints live under their own prefix and authenticate with
	// a scoped SDK key rather than a dashboard session.
	r.Mount("/sdk/v1", api.SDKRouter())

	return r
}

// writeJSON writes a JSON response with the correct content type.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := encodeJSON(w, body); err != nil {
		slog.Error("write json response", "error", err)
	}
}
