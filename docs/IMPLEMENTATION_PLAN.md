# IMPLEMENTATION_PLAN.md

## Lightweight Feature Flag Platform

**Status:** MVP delivery plan\
**Execution model:** Monorepo, independently buildable
dashboard/API/SDK\
**Backend:** Go modular monolith\
**Database:** Supabase PostgreSQL

------------------------------------------------------------------------

## 1. Delivery principles

1.  Ship the smallest complete feature-flag workflow.
2.  Keep the backend as one Go application with clear internal modules.
3.  Use PostgreSQL as the source of truth and in-memory snapshots for
    runtime reads.
4.  Implement SSE as notification delivery; SDK fetches the
    authoritative snapshot.
5.  Keep Supabase as a PostgreSQL provider, not an application framework
    dependency.
6.  Add infrastructure only when a real requirement appears.
7.  Test isolation and correctness before optimizing throughput.

## 2. MVP deliverables

-   Next.js simple admin dashboard.
-   Go REST API and SSE endpoint.
-   Supabase PostgreSQL schema and migrations.
-   Custom dashboard authentication and session management.
-   Project and default environment management.
-   Boolean feature flag management.
-   Environment-specific ON/OFF state.
-   Project/environment-scoped SDK credentials.
-   TypeScript SDK with local evaluation and SSE synchronization.
-   Docker packaging, health checks, and deployment documentation.

## 3. Milestones

### M0 --- Repository and development foundation

**Tasks** - Create monorepo directories for dashboard, API, SDK, docs,
and infrastructure. - Initialize the Git repository with a complete
`.gitignore` covering secrets, Node, Go, and Next.js artifacts. - Add
`.env.example` with placeholders for every configuration value, and
verify no real secret is tracked. - Initialize Next.js dashboard. -
Initialize Go module and HTTP server using `net/http` and `chi`. - Add
root Makefile and local development commands. - Add environment variable
examples without real secrets. - Add formatting, linting, and test
commands. - Add Dockerfiles for dashboard and API.

**Acceptance criteria** - Dashboard and API run independently. -
`go test ./...` runs. - Dashboard lint/build commands run. - No secrets
are committed, and `.gitignore` excludes `.env`,
`.claude/settings.local.json`, and any file containing a live token. -
Tracked config files use placeholders or environment references only. -
README explains local setup.

### M1 --- PostgreSQL schema and core management API

**Tasks** - Provision Supabase PostgreSQL project and select region
close to the API deployment region. - Configure runtime and migration
database connections. - Establish the test database strategy
(`TECHNICAL_SPEC.md` §15.1): migrations applied at test start, per-test
unique schema, dedicated test role. - Create the runtime and migration
roles with least privilege, and verify the runtime role cannot create
schemas. - Add migrations for users, sessions, projects, environments,
feature_flags, feature_flag_states, and sdk_keys. - Add foreign keys,
unique constraints, indexes, and delete behavior exactly as specified
in `TECHNICAL_SPEC.md` §4. - Enforce same-project integrity with
composite foreign keys (`feature_flag_states` and `sdk_keys`), not
application checks alone. - Add `projects.owner_id`. - Configure `sqlc`
and `pgxpool`. - Implement project create/list/get/update/delete. - Create
Development, Staging, and Production environments transactionally with
project creation. - Implement flag create/list/update-description/delete.
- Implement idempotent ON/OFF state update per environment using the
lock order in `TECHNICAL_SPEC.md` §6.2. - Build snapshots from
in-transaction data (§6.4). - Add service-level project/environment
ownership validation.

**Acceptance criteria** - Migrations can be applied to a clean
database. - Project creation creates exactly the three default
environments. - Duplicate project slug and flag key are rejected
cleanly. - A flag has independent state in each environment. -
Cross-project flag/environment combinations are rejected **by a
database constraint**, verified by a test that attempts the insert
directly against PostgreSQL. - A no-op write does not increment
`config_version`. - Flag creation increments all three environment
versions. - Integration tests use PostgreSQL, not only mocks.

### M2 --- Custom dashboard authentication and simple dashboard

**Tasks** - Implement secure initial-admin bootstrap command or one-time
setup process. - Implement Argon2id password hashing and verification. -
Implement opaque random session tokens; store only token hashes. -
Implement login, logout, current-user endpoints, session expiry, and
revocation. - Configure Secure/HttpOnly/SameSite cookies for
production. - Add CSRF protection for cookie-authenticated mutations. -
Add login throttling and generic error responses. - Build login
screen. - Build project list/create/edit screens. - Build project detail
with environment selector. - Build feature flag list/create/edit and
per-environment ON/OFF controls. - Build SDK key create/revoke UI.

**Acceptance criteria** - Unauthenticated users cannot access management
endpoints. - Session is revoked on logout and expires correctly. -
Password and session token are never returned or logged. - Dashboard
supports the complete management flow. - No public sign-up or complex
RBAC is introduced.

### M3 --- Configuration service and in-memory cache

**Tasks** - Define the SDK configuration response contract
(`TECHNICAL_SPEC.md` §5). - Implement configuration snapshot builder for
a project/environment. - Load all required snapshots at API startup
before readiness. - Implement concurrency-safe immutable snapshot
storage. - Implement SDK config endpoint with scoped credential
validation. - Update the affected snapshot after a committed flag
change, using the in-transaction build rule (§6.4). - Implement version
increments transactionally and serialize updates per environment with
the fixed lock order (§6.2). - Implement the idempotent no-op rule
(§6.3). - Handle cache refresh failures and degraded readiness.

**Acceptance criteria** - SDK config endpoint reads from memory, not
PostgreSQL on every request. - Snapshot contains only Boolean flag
values and safe metadata. - Snapshot version matches the environment
configuration version **and the version always corresponds to the
content in the same response**. - Concurrent updates do not lose
version increments, and the final version equals the number of
effective changes applied. - A no-op write does not increment the
version. - Restart reloads correct state from PostgreSQL. -
`go test -race ./...` passes, including a test that reads snapshots
concurrently while writers mutate them.

### M4 --- SSE and TypeScript SDK

**Tasks** - Implement SSE connection lifecycle and scoped
authentication using the `FF` authorization scheme (§7.5). - Implement
`config.updated` version notification and heartbeat, with the event
`id` equal to the environment's `config_version`. - Add bounded
subscriber channels and slow-client disconnect behavior. - Implement
graceful shutdown and context cancellation. - Implement immediate
revocation signaling for active streams on key revocation (§8). -
Implement TypeScript SDK initialization and initial snapshot fetch. -
Implement local `isEnabled(key, fallback)` evaluation. - Implement
change subscriptions. - Implement SSE streaming with `fetch()` and a
stream reader, never the browser `EventSource` API, so the
`Authorization` header can be sent. - Implement reconnect with
exponential backoff and jitter. - Fetch latest snapshot on reconnect
and atomically replace local state. - Retry a failed snapshot fetch
independently of event arrival. - Implement SDK `close()` and
connection status.

**Acceptance criteria** - Toggling a flag updates connected SDK clients
without redeployment or page reload. - SDK does not make a network
request during local flag evaluation, verified with a counting
transport stub. - SDK retains last valid configuration during
temporary disconnection. - SDK catches up after reconnect even if
intermediate events were missed. - A failed snapshot fetch is retried
even when no further event arrives. - Slow SSE consumers cannot block
flag mutations. - Revoked SDK credentials are rejected and their active
streams are closed immediately on a single instance.

### M5 --- Hardening, release, and operational readiness

**Tasks** - Add request IDs and structured logging. - Add health and
readiness endpoints. - Add database pool limits and timeouts. - Add API
and SDK rate/connection limits. - Review CORS, CSRF, cookie, TLS, and
secret handling. - Add security tests for cross-project and
cross-environment isolation. - Add load tests for config fetch and
concurrent SSE connections. - Verify reverse proxy SSE buffering and
idle timeout settings. - Configure database backups and recovery
expectations in Supabase. - Document deployment, environment variables,
migrations, key rotation, and recovery. - Add CI jobs that run only the
affected application/package checks where practical.

**Acceptance criteria** - Production configuration contains no
development credentials. - Backup and restore procedure is documented
and tested to the level supported by the chosen Supabase plan. - SSE
works through the production ingress/proxy. - Load test results and
tested limits are documented. - Release checklist is complete.

## 4. Suggested implementation order

Within each milestone, use this dependency order:

1.  Database schema and migration.
2.  Domain/service logic.
3.  HTTP handlers and API contract.
4.  Integration/unit tests.
5.  Dashboard or SDK integration.
6.  End-to-end acceptance test.

Avoid building all dashboard screens before the API contract stabilizes.

## 5. Test plan

### Backend unit tests

-   Flag key validation.
-   Default OFF state creation.
-   Environment isolation rules.
-   Idempotent state update.
-   Version increment rules.
-   Cache snapshot replacement.
-   Session creation, expiry, and revocation.
-   SDK key parsing and verification.

### Backend integration tests

-   Project creation and default environments.
-   CRUD operations.
-   Duplicate key constraints.
-   Transaction rollback.
-   Cross-project access rejection.
-   Cross-project flag/environment state insert rejected by a database
    constraint, attempted directly against PostgreSQL.
-   Environment-specific flag states.
-   SDK config response scope.
-   Revoked SDK key rejection.
-   Delete cascading behavior for project, flag, and environment.

### Realtime tests

-   SSE connection authentication.
-   Correct project/environment subscriber routing.
-   Version event after committed change.
-   No event before transaction commit.
-   Heartbeat.
-   Client disconnect and cleanup.
-   Slow consumer handling.
-   Reconnect and snapshot catch-up.
-   Multiple simultaneous updates.

### SDK tests

-   Initial fetch.
-   Local evaluation.
-   Unknown key fallback.
-   Event subscription.
-   Atomic snapshot replacement.
-   Reconnect/backoff.
-   Last-known-good behavior.
-   Close/cleanup.
-   No server secret included in browser build.

### Concurrency and performance

-   Run `go test -race ./...`.
-   Concurrent updates to the same environment preserve version
    monotonicity **and** version/content correspondence.
-   Snapshot reads run concurrently with writes, so the race detector
    covers the shared-state surface.
-   Benchmark in-memory `isEnabled` path in SDK.
-   Load-test config endpoint.
-   Load-test concurrent SSE connections with realistic heartbeat and
    update rates.
-   Measure CPU, memory, database connections, propagation latency, and
    error rates.
-   Do not publish capacity claims without recorded test conditions.

### SDK transport assertions

-   `isEnabled` performs no network I/O, verified by replacing the
    transport with a counting stub and asserting a zero call count.
-   A failed snapshot fetch is retried even when no further SSE event
    arrives.
-   The browser build path uses `fetch()` streaming, never `EventSource`,
    so the `Authorization` header is always sent.

## 6. CI/CD plan

Minimum CI stages: 1. Formatting and static checks. 2. Go unit tests. 3.
Go race tests (may be a separate job). 4. PostgreSQL
migration/integration tests. 5. Dashboard lint and build. 6. SDK tests
and build. 7. Docker image build. 8. Deployment on approved branch/tag.

The integration-test stage requires a live PostgreSQL instance. It uses
a dedicated test database and the migration connection string, applying
migrations at the start of the run and using per-test schemas
(`TECHNICAL_SPEC.md` §15.1). The stage must not run against a shared or
production database, and it must not use the read-only Supabase MCP
connection.

Monorepo optimization: - Dashboard changes trigger dashboard
checks/build. - API changes trigger Go checks/build. - SDK changes
trigger SDK checks/build. - Shared docs changes do not need application
builds. - Changes to database schema trigger API integration tests.

## 7. Environment and configuration management

Use separate configuration values for: - Dashboard public API base
URL. - Go API listen address. - Runtime PostgreSQL connection string. -
Migration PostgreSQL connection string. - Session cookie settings. -
Initial administrator bootstrap input. - Allowed dashboard origins. -
Rate and connection limits.

Keep secrets in deployment secret storage. Commit only `.env.example`
with placeholders.

### 7.1 Supabase project access

The development project is `toctqvfglwuvvnovkmsc`.

**Verified.** The project is provisioned and reachable:
`db.toctqvfglwuvvnovkmsc.supabase.co` and
`aws-0-toctqvfglwuvvnovkmsc.pooler.supabase.com` both resolve, TCP
port 5432 on the direct host accepts connections, and the project's API
tier responds with `401 Invalid API key` (expected without a key). The
project is neither paused nor deleted.

The database was inspected directly. It is **empty**: no tables in
`public`, no `schema_migrations` table, and no migrations previously
applied. `pgcrypto` is already installed, so `gen_random_uuid()` is
available without a separate extension step. The server is PostgreSQL
17.12 with `max_connections = 60`.

### 7.2 Database roles

Two roles exist, created and verified against the development project.
The runtime role is used by the API; the migration role owns the schema.

| Role | Purpose | Privileges |
| ---- | ------- | ---------- |
| `ff_migrator` | Migrations | Owns schema `public`; `CREATE` on the database |
| `ff_runtime` | Application runtime | `USAGE` on `public`; DML on tables and sequences. **No `CREATE`.** |

`ff_runtime` deliberately cannot create a schema or a table. A test
asserts this.

The setup required two non-obvious steps, because Supabase restricts the
`public` schema and default privileges:

1. **The `public` schema must be reassigned to `ff_migrator`.** Supabase
   assigns `public` to `pg_database_owner`, which leaves the migration
   role unable to create tables. Ownership transfer requires the
   executing role to be a member of the target role:

   ``` sql
   GRANT ff_migrator TO postgres;
   ALTER SCHEMA public OWNER TO ff_migrator;
   ```

2. **Default privileges must be set `FOR ROLE ff_migrator`.** Setting
   them as `postgres` has no effect, because `ff_migrator` is the role
   that actually creates the tables:

   ``` sql
   ALTER DEFAULT PRIVILEGES FOR ROLE ff_migrator IN SCHEMA public
     GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO ff_runtime;
   ALTER DEFAULT PRIVILEGES FOR ROLE ff_migrator IN SCHEMA public
     GRANT USAGE, SELECT ON SEQUENCES TO ff_runtime;
   ```

Without step 2, migrations create tables that `ff_runtime` cannot read.

The full applied setup is:

``` sql
CREATE ROLE ff_runtime LOGIN PASSWORD '<generated>';
CREATE ROLE ff_migrator LOGIN PASSWORD '<generated>';

GRANT ff_migrator TO postgres;
GRANT CONNECT ON DATABASE postgres TO ff_runtime, ff_migrator;
GRANT ALL ON DATABASE postgres TO ff_migrator;

ALTER SCHEMA public OWNER TO ff_migrator;
GRANT ALL ON SCHEMA public TO ff_migrator;

GRANT USAGE ON SCHEMA public TO ff_runtime;
REVOKE CREATE ON SCHEMA public FROM ff_runtime;

ALTER DEFAULT PRIVILEGES FOR ROLE ff_migrator IN SCHEMA public
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO ff_runtime;
ALTER DEFAULT PRIVILEGES FOR ROLE ff_migrator IN SCHEMA public
  GRANT USAGE, SELECT ON SEQUENCES TO ff_runtime;
```

Role passwords are generated and stored only in the local, gitignored
`.env`. They are never committed. The runtime role password does not
need to be known to anyone but the developer running the API.

### 7.3 Connection strings

`.env` holds both connection strings:

``` dotenv
DATABASE_URL=postgresql://ff_runtime:<pw>@db.toctqvfglwuvvnovkmsc.supabase.co:5432/postgres?sslmode=require
MIGRATION_DATABASE_URL=postgresql://ff_migrator:<pw>@db.toctqvfglwuvvnovkmsc.supabase.co:5432/postgres?sslmode=require
```

The direct IPv6 host was confirmed reachable from the development
machine. If the deployment host has no IPv6 route, switch to the
region-specific pooler host shown in the Supabase dashboard's connection
string dropdown. The pooler hostname embeds the region, so it must be
copied verbatim rather than constructed by hand.

### 7.4 Optional: Supabase MCP access

The Supabase MCP server is configured `read_only=true` and requires an
interactive OAuth login. It is useful for read-only schema inspection
and documentation, but it **cannot** create schemas or apply
migrations. Schema changes always go through `golang-migrate` over the
`MIGRATION_DATABASE_URL` connection.

Do not expose database URLs, password hashes, session secrets, or server
SDK keys to Next.js public environment variables.

## 8. MVP release checklist

-   [ ] Product requirements and API contracts reviewed.
-   [ ] Database migrations applied successfully.
-   [ ] Runtime database role has minimum required permissions.
-   [ ] Initial administrator bootstrap completed securely.
-   [ ] Dashboard login/session/CSRF behavior verified.
-   [ ] Project and environment management verified.
-   [ ] Flag CRUD and ON/OFF behavior verified.
-   [ ] Environment and project isolation verified.
-   [ ] SDK key creation, display-once, and revocation verified.
-   [ ] SDK config endpoint returns correct snapshot/version.
-   [ ] SSE notifications and reconnect verified.
-   [ ] SDK local evaluation and fallback verified.
-   [ ] Race tests pass.
-   [ ] Load test conditions and results documented.
-   [ ] HTTPS and proxy SSE settings verified.
-   [ ] Database backup/recovery policy confirmed.
-   [ ] Logs contain no credentials or sensitive tokens.
-   [ ] Deployment and rollback instructions documented.

## 9. Post-MVP candidates (not commitments)

Prioritize these only after observing actual usage: - Multiple users and
project-level roles. - Organizations/tenancy. - Audit history and
rollback. - Custom environments. - Flag archival and lifecycle
policies. - JSON or typed configuration. - Targeting and percentage
rollout. - Cross-instance event propagation with PostgreSQL NOTIFY or
Redis Pub/Sub. - Dedicated stream service if operational measurements
justify it.

## 10. Delivery guardrails

Do not add the following to MVP without a demonstrated requirement: -
Microservices. - Redis or Kafka. - WebSockets. - Kubernetes-specific
infrastructure. - Experimentation or analytics. - User targeting or
segmentation. - Complex RBAC. - Multi-region replication. - Custom
query/ORM abstractions beyond what the codebase needs.

The MVP is successful when an administrator can create a Boolean flag,
set it independently per environment, and see connected SDK clients
receive the new value reliably and securely.
