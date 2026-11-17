# Feature Flag Platform

A lightweight feature flag platform. An administrator manages Boolean
flags per project and environment in a web dashboard, and connected
applications receive configuration changes in real time without a
redeploy.

This is deliberately **not** an experimentation platform. There are no
A/B tests, no user targeting, no percentage rollouts, and no analytics.
The scope is the core flag lifecycle, done properly.

## What it does

- **Projects** own environments and flags. Creating a project also
  creates its `development`, `staging`, and `production` environments.
- **Boolean flags** belong to one project. Each environment holds its
  own ON or OFF state, so a flag can be on in staging and off in
  production.
- **SDK keys** are scoped to one project and environment. A key for
  production cannot read staging.
- **Configuration changes** are versioned. Each environment has a
  monotonically increasing version, and a change that is persisted
  increments it.
- **Connected applications** receive a version notification over SSE and
  fetch the new configuration. Evaluation itself is local and never
  performs a network request.

## Architecture

```
apps/
  api/          Go API: authentication, configuration, SSE delivery
  dashboard/    Next.js admin interface
  demo-app/     A small application that shows the SDK in use
packages/
  sdk/          Framework-agnostic TypeScript SDK
infrastructure/
  docker/       Container images and the compose stack
docs/           Product, technical, and implementation specifications
```

The API is a Go modular monolith. PostgreSQL, hosted by Supabase, is the
source of truth. Runtime reads come from an in-memory snapshot cache.
SSE is notification delivery only; the SDK fetches the authoritative
configuration.

The dashboard, the API, and the SDK are separate applications in one
repository. Each builds and deploys independently.

| Document | Contents |
| -------- | -------- |
| [docs/PRODUCT_SPEC.md](docs/PRODUCT_SPEC.md) | Scope, requirements, acceptance criteria |
| [docs/TECHNICAL_SPEC.md](docs/TECHNICAL_SPEC.md) | Architecture, schema, API, SSE, security |
| [docs/IMPLEMENTATION_PLAN.md](docs/IMPLEMENTATION_PLAN.md) | Milestones, tests, release checklist |
| [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) | Toolchain setup and its Windows specifics |

## Requirements

| Tool | Version | Notes |
| ---- | ------- | ----- |
| Go | 1.27.4 | The API and the migrations |
| Node.js | 24.x | The dashboard, the SDK, and the demo app |
| PostgreSQL | 15+ | Provided by Supabase |
| Docker | Any | Optional, for the container stack |

The Go race detector needs a C compiler. On Windows, see
[docs/DEVELOPMENT.md](docs/DEVELOPMENT.md).

## Getting started

### 1. Configure the environment

```bash
cp .env.example .env
```

Fill in the two connection strings. They use different roles on purpose:

```dotenv
# DML only. The API uses this one.
DATABASE_URL=postgresql://ff_runtime:<password>@db.<project-ref>.supabase.co:5432/postgres?sslmode=require

# Owns the schema. Migrations use this one.
MIGRATION_DATABASE_URL=postgresql://ff_migrator:<password>@db.<project-ref>.supabase.co:5432/postgres?sslmode=require
```

Also set a `CSRF_SECRET` of at least 32 bytes:

```bash
openssl rand -base64 32
```

The API refuses to start without it.

The two roles are created once, in the Supabase SQL editor. The
statements are in
[docs/IMPLEMENTATION_PLAN.md](docs/IMPLEMENTATION_PLAN.md) §7.2.

### 2. Apply the migrations

```bash
make migrate-up
```

### 3. Create the first administrator

There is no public sign-up. The first administrator is created from the
command line:

```bash
cd apps/api
BOOTSTRAP_ADMIN_EMAIL=admin@example.com \
BOOTSTRAP_ADMIN_PASSWORD='a-strong-password' \
go run ./cmd/admin bootstrap
```

Remove the password from the environment afterwards.

### 4. Run it

With Go and Node installed locally:

```bash
# Terminal 1: the API
make api-run

# Terminal 2: the dashboard
cd apps/dashboard && npm install && npm run dev
```

| Service | URL |
| ------- | --- |
| Dashboard | http://localhost:3000 |
| API | http://localhost:8080 |

### Or run it in containers

```bash
cd infrastructure/docker
cp ../../.env .env
docker compose up --build -d
```

This also runs the demo application on port 4000. It needs an SDK key:

```dotenv
DEMO_SDK_KEY=ff_client_...
```

See [infrastructure/docker/README.md](infrastructure/docker/README.md)
for the details, including the IPv6 requirement for reaching Supabase
from a container.

## Using the SDK

```ts
import { createFeatureFlagClient } from "@featureflag/sdk";

const client = createFeatureFlagClient({
  apiUrl: "https://api.example.com",
  sdkKey: "ff_client_...",
});

// Fetches the first snapshot before resolving.
await client.initialize();

// Reads local memory. This never performs a network request.
const enabled = client.isEnabled("new_dashboard", false);

// Called after a new snapshot is applied.
const unsubscribe = client.on("change", (snapshot) => {
  console.log("configuration is now at version", snapshot.version);
});

// Stops the stream and releases resources.
await unsubscribe();
await client.close();
```

`isEnabled(key, fallback)` returns the fallback when the flag is absent,
so a missing flag and a flag set to `false` are different things.

### How the SDK stays current

The SDK holds the configuration in memory and reads only from it. The
update stream is a notification: when the version changes, the SDK
fetches the new snapshot and replaces the old one atomically, then
notifies subscribers.

Two details matter in practice:

- A failed fetch is retried until it succeeds, **independently of
  whether another event arrives**. Otherwise a client could sit on a
  stale snapshot while appearing connected.
- The stream uses `fetch()` with a streaming body reader, not the
  browser `EventSource` API, because `EventSource` cannot send the
  `Authorization` header the API requires.

### Credential types

| Type | Visibility | Use |
| ---- | ---------- | --- |
| `client` | Visible to anyone using the application | Browser and mobile apps |
| `server` | Confidential | Backend services |

Both are read-only and scoped to one project and environment. A server
key must never be included in a browser bundle.

## The API

### Dashboard endpoints

All require a session cookie. State-changing requests also require the
`X-CSRF-Token` header.

| Method | Path | Purpose |
| ------ | ---- | ------- |
| `POST` | `/api/v1/auth/login` | Sign in |
| `POST` | `/api/v1/auth/logout` | Sign out |
| `GET` | `/api/v1/auth/me` | Current administrator |
| `GET` | `/api/v1/projects` | List projects |
| `POST` | `/api/v1/projects` | Create a project and its environments |
| `GET` | `/api/v1/projects/{projectID}` | Project detail |
| `PATCH` | `/api/v1/projects/{projectID}` | Update a project |
| `DELETE` | `/api/v1/projects/{projectID}` | Delete a project |
| `GET` | `/api/v1/projects/{projectID}/environments` | List environments |
| `GET` | `/api/v1/projects/{projectID}/flags?environment={id}` | List flags with their state |
| `POST` | `/api/v1/projects/{projectID}/flags` | Create a flag |
| `PATCH` | `/api/v1/flags/{flagID}` | Update a description |
| `DELETE` | `/api/v1/flags/{flagID}` | Delete a flag |
| `PUT` | `/api/v1/flags/{flagID}/environments/{environmentID}` | Set a flag's state |
| `GET` | `/api/v1/sdk-keys?project_id=&environment=` | List SDK keys |
| `POST` | `/api/v1/sdk-keys` | Issue an SDK key |
| `DELETE` | `/api/v1/sdk-keys/{keyID}?project_id=` | Revoke an SDK key |

### SDK endpoints

| Method | Path | Purpose |
| ------ | ---- | ------- |
| `GET` | `/sdk/v1/config` | Fetch the configuration snapshot |
| `GET` | `/sdk/v1/stream` | Subscribe to change notifications |

Both authenticate with `Authorization: FF <sdk-key>`. The `Bearer` scheme
is not accepted.

The configuration response:

```json
{
  "project_id": "uuid",
  "environment": "production",
  "version": 42,
  "flags": {
    "new_dashboard": true,
    "new_checkout": false
  }
}
```

It contains only flag keys, Boolean values, and the metadata the SDK
needs. No user data, no session information, no credentials.

### Operational endpoints

| Method | Path | Purpose |
| ------ | ---- | ------- |
| `GET` | `/healthz` | Process liveness |
| `GET` | `/readyz` | Database connectivity and snapshot cache |

## Development

```bash
make help              # List every target
make api-test          # Go tests
make api-test-race     # Go tests with the race detector
make sdk-test          # SDK tests
make dashboard-build   # Build the dashboard
make check             # Formatting, vet, and race tests
```

### How the configuration stays consistent

This is the part worth understanding before changing the code.

A flag change is a single database transaction:

1. Lock the environment's row with `SELECT ... FOR UPDATE`.
2. Read the current state.
3. If the requested value already matches, stop. Nothing changed, so no
   version increment and no event.
4. Write the state and increment the environment's version in the same
   transaction.
5. Build the snapshot from the values read and written in that
   transaction.
6. Commit.
7. Install the snapshot in memory.
8. Publish the version over SSE.

Two properties follow from this, and both are tested:

- **The environment lock is always taken first.** Locking the state row
  instead would not serialise the version increment against the state
  update, and the two could interleave.
- **A snapshot's version always describes its own content.** The
  snapshot is built from the transaction, never by re-querying after the
  commit, so the two cannot disagree.

Writing a value that is already set is a no-op, so the `PUT` endpoint is
safe to retry.

### Project isolation

A flag and an environment may only be paired when they belong to the
same project. This is enforced by the database, with composite foreign
keys, rather than only by application code:

```sql
FOREIGN KEY (flag_id, project_id)
  REFERENCES feature_flags (id, project_id) ON DELETE CASCADE
```

A cross-project row is not representable. The integration tests assert
this by attempting the insert directly against PostgreSQL.

## Testing

```bash
make api-test-race   # Go unit and integration tests, with the race detector
make sdk-test        # SDK tests
```

The Go integration tests run against a real PostgreSQL database. Each
test creates its own schema, applies the migrations there, and drops the
schema afterwards, so the tests are independent and can run
concurrently.

They need a connection string:

```bash
export TEST_DATABASE_URL="$MIGRATION_DATABASE_URL"
```

The migration role is required because the test harness creates
schemas. The test database must never be a production database.

The dashboard has a browser test that drives the real interface:

```bash
cd apps/dashboard
npx playwright install chromium
npm run e2e
```

It needs the API and the dashboard running, plus an administrator's
credentials in `ADMIN_EMAIL` and `ADMIN_PASSWORD`.

## Security

- Passwords use Argon2id. Session tokens and SDK key secrets use
  SHA-256, because they are high entropy random values where a slow key
  derivation would cost on every request without protecting against an
  offline attacker.
- Only hashes of session tokens and SDK keys are stored. A session token
  is delivered in a Secure, HttpOnly cookie. The CSRF token is bound to
  the session and is readable by the dashboard's own JavaScript, which
  is what the double submit mechanism requires.
- SDK keys are shown once, at creation. They can be revoked
  permanently, and revoking one closes its active streams immediately.
- The runtime database role cannot modify the schema. Migrations use a
  separate role.
- Passwords, session tokens, and raw SDK keys are never logged, and
  never returned to a browser.

## Deployment

The API and the dashboard deploy independently as containers. See
[infrastructure/docker/README.md](infrastructure/docker/README.md) for
the details on TLS, the reverse proxy, SSE buffering, and applying
migrations.

The API runs as a **single instance**. The configuration cache and the
SSE subscriber registry are in process, so a second instance would not
see changes made through the first. Cross-instance propagation is
documented as a growth path, not an MVP feature.

## License

MIT
