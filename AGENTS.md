# Repository-wide Agent Instructions

These instructions apply to every coding agent working in this repository.

## Product invariants
- This is a lightweight feature-flag platform, not an experimentation platform.
- MVP supports Boolean flags only.
- A flag definition belongs to one project; its ON/OFF state is independent per environment.
- Initial environments are Development, Staging, and Production.
- No A/B testing, experimentation, analytics, user targeting, segments, percentage rollout, or arbitrary JSON flags in MVP.
- Do not expand product scope without explicit approval.

## Architecture invariants
- Monorepo with independently buildable/deployable applications:
  - `apps/dashboard`: Next.js dashboard.
  - `apps/api`: Go modular monolith.
  - `packages/sdk`: framework-agnostic TypeScript SDK.
- Supabase is used as a managed PostgreSQL provider only.
- Do not introduce Supabase Auth, Supabase Realtime, Edge Functions, or business logic tied to Supabase APIs.
- PostgreSQL is the source of truth.
- Backend in-memory configuration snapshots serve runtime reads.
- SSE is notification delivery; the SDK fetches the authoritative configuration snapshot.
- SDK flag evaluation is local and must not perform a network request.
- Keep the Go backend as a modular monolith. Do not introduce microservices.

## Security invariants
- Never expose database credentials, session tokens, password hashes, raw SDK keys, or server SDK credentials to browser code or logs.
- Dashboard authentication is custom, session-based, and server-managed.
- Store only hashes of opaque session tokens and SDK keys.
- SDK credentials are read-only and scoped to one project/environment.
- Enforce project/environment ownership on the server; never trust IDs supplied by a client without authorization checks.
- Protect cookie-authenticated mutations against CSRF.
- Treat browser client SDK keys as public identifiers with restricted read-only access, not as secrets.
- Destructive database operations require explicit human approval and must never be inferred from a task.

## Engineering rules
- Read relevant files under `docs/` before implementing or changing architecture.
- Prefer the simplest implementation that satisfies the documented requirements.
- Keep domain logic independent from HTTP handlers and database transport.
- Use context cancellation, bounded queues, explicit timeouts, and graceful shutdown for long-lived/concurrent work.
- Persist a configuration change and increment its version in one transaction. Never publish an SSE event before commit.
- Use immutable/concurrency-safe snapshots. Run race tests for concurrency-sensitive changes.
- Use idempotent desired-state APIs for flag updates (`enabled: true|false`), not blind toggle endpoints.
- Add or update tests with every behavior change.
- Do not claim tests, builds, migrations, or security checks passed unless they were actually run and their results inspected.

## Required completion report
At the end of a task, report:
1. What changed.
2. Files changed.
3. Tests/checks actually run and their results.
4. Known gaps, risks, or follow-up work.
5. Any migration or operational action requiring human approval.
