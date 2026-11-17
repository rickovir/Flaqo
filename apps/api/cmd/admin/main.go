// Command admin creates and manages the initial administrator account.
//
// The MVP has no public registration. The first administrator is created
// with this command, which reads the email and password from the
// environment so they never appear in a shell history or process list.
//
// Usage:
//
//	go run ./cmd/admin bootstrap
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/featureflag/api/internal/auth"
	"github.com/featureflag/api/internal/config"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		return errors.New("usage: admin bootstrap")
	}

	switch os.Args[1] {
	case "bootstrap":
		return bootstrap()
	default:
		return fmt.Errorf("unknown command %q", os.Args[1])
	}
}

func bootstrap() error {
	email := os.Getenv("BOOTSTRAP_ADMIN_EMAIL")
	password := os.Getenv("BOOTSTRAP_ADMIN_PASSWORD")

	if email == "" || password == "" {
		return errors.New("BOOTSTRAP_ADMIN_EMAIL and BOOTSTRAP_ADMIN_PASSWORD must both be set")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	connectCtx, cancel := context.WithTimeout(ctx, cfg.DatabaseConnectTimeout)
	defer cancel()

	pool, err := pgxpool.New(connectCtx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(connectCtx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	svc := auth.NewService(pool, auth.DefaultArgon2Params(), cfg.CSRFSecret,
		cfg.SessionAbsoluteTTL, cfg.SessionIdleTTL, nil)

	userID, err := svc.CreateUser(ctx, email, password)
	if err != nil {
		return fmt.Errorf("create administrator: %w", err)
	}

	// The email is safe to print. The password is never echoed.
	fmt.Printf("administrator created: id=%s email=%s\n", userID, auth.NormalizeEmail(email))
	fmt.Println("remove BOOTSTRAP_ADMIN_PASSWORD from the environment now")
	return nil
}
