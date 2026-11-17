package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"
)

// runHealthcheck is a separate entry point for the container health
// check.
//
// It is compiled into the same binary and selected by a flag, so the
// image needs no extra tooling such as curl or wget, which the
// distroless base image does not have.
//
// It exits 0 when the API is ready and 1 otherwise.
func runHealthcheck() error {
	base := os.Getenv("HEALTHCHECK_URL")
	if base == "" {
		base = "http://127.0.0.1:8080"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/readyz", nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("health check request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned status %d", resp.StatusCode)
	}
	return nil
}
