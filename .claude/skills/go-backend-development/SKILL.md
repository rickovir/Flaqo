---
name: go-backend-development
description: Use when implementing, refactoring, testing, or reviewing Go API code, services, handlers, database access, concurrency, or application structure.
---

# Go Backend Development Workflow

1. Read `AGENTS.md`, the Go rules, and relevant technical specification sections.
2. Inspect existing package patterns before adding new packages or abstractions.
3. Keep the modular monolith boundaries clear; avoid microservices and speculative interfaces.
4. Keep handlers thin: parse/validate transport input, call application/domain logic, map errors to HTTP.
5. Pass contexts and define bounded timeouts for external operations.
6. Use `pgxpool` and generated `sqlc` queries where configured; parameterize SQL.
7. Make goroutine ownership and shutdown explicit. Every long-lived goroutine needs a cancellation/termination path.
8. Add unit and integration tests with the change.
9. Run `gofmt`, targeted tests, `go test ./...`, and `go test -race ./...` when concurrency is affected.
10. Report commands actually run and any unverified behavior.
