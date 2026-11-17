// Package database owns the PostgreSQL connection pool and exposes the
// small surface the rest of the API needs.
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/featureflag/api/internal/config"
)

// Pool wraps pgxpool with the settings the API depends on.
type Pool struct {
	*pgxpool.Pool
}

// Connect opens the pool and verifies connectivity.
//
// The pool is bounded: max_conns is a hard ceiling, so a traffic spike
// queues rather than opening unbounded connections to PostgreSQL.
func Connect(ctx context.Context, cfg *config.Config) (*Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}

	poolCfg.MaxConns = cfg.DatabaseMaxConns
	poolCfg.MinConns = cfg.DatabaseMinConns
	poolCfg.MaxConnLifetime = 30 * time.Minute
	poolCfg.MaxConnIdleTime = 5 * time.Minute
	poolCfg.HealthCheckPeriod = 30 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, cfg.DatabaseConnectTimeout)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return &Pool{Pool: pool}, nil
}

// WithQueryTimeout derives a context bounded by the configured query
// timeout. Callers use it for individual queries that need a tighter
// deadline than the request context.
func (p *Pool) WithQueryTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, timeout)
}

// Stats reports pool usage for the readiness endpoint and metrics.
func (p *Pool) Stats() map[string]int32 {
	s := p.Stat()
	return map[string]int32{
		"acquired_conns": int32(s.AcquiredConns()),
		"idle_conns":     int32(s.IdleConns()),
		"total_conns":    int32(s.TotalConns()),
		"max_conns":      int32(s.MaxConns()),
	}
}
