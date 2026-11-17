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

	"github.com/featureflag/api/internal/config"
	"github.com/featureflag/api/internal/database"
	"github.com/featureflag/api/internal/middleware"
)

func main() {
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

	router := buildRouter(cfg, pool, snapshots, logger)

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

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	logger.Info("server stopped")
	return nil
}

// buildRouter assembles the HTTP handler.
func buildRouter(cfg *config.Config, pool *database.Pool, snapshots *config.Store, logger *slog.Logger) http.Handler {
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

	r.Route("/api/v1", func(r chi.Router) {
		// Dashboard and SDK routes are added as the modules are implemented.
		r.NotFound(func(w http.ResponseWriter, req *http.Request) {
			writeJSON(w, http.StatusNotFound, map[string]any{
				"error": map[string]string{
					"code":    "not_found",
					"message": "endpoint not found",
				},
			})
		})
	})

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
