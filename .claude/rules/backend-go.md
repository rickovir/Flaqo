---
paths:
  - "apps/api/**/*.go"
  - "apps/api/go.mod"
  - "apps/api/go.sum"
---

# Go Backend Rules

- Prefer standard library packages and small dependencies.
- HTTP server uses `net/http`; routing uses `chi`.
- PostgreSQL access uses `pgx/v5` and generated `sqlc` queries where configured.
- Pass `context.Context` through request-scoped operations; do not store request contexts in structs.
- Wrap errors with context and map domain errors to stable HTTP responses at the handler boundary.
- Do not panic for expected runtime errors.
- Close rows, statements, bodies, and resources; check iteration errors.
- Bound database pools, request timeouts, SSE subscriber queues, and goroutine lifetimes.
- Avoid shared mutable maps without synchronization.
- Run `go test -race ./...` for concurrency-sensitive changes.
- Run `gofmt` on changed Go files.
