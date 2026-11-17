# TECHNICAL_SPEC.md

## Lightweight Feature Flag Platform

**Status:** MVP technical baseline\
**Architecture:** Monorepo + modular monolith\
**Backend:** Go\
**Database:** Supabase-hosted PostgreSQL\
**Realtime:** Server-Sent Events (SSE)\
**SDK:** TypeScript

------------------------------------------------------------------------

## 1. Architecture decisions

  Area                Decision
  ------------------- --------------------------------------------------------
  Repository          Monorepo
  Frontend            Next.js dashboard
  Backend             Go modular monolith
  HTTP                `net/http` + `chi` router
  Database            Supabase PostgreSQL
  PostgreSQL driver   `pgx/v5`
  SQL generation      `sqlc`
  Migration           `golang-migrate`
  Cache               In-memory immutable configuration snapshots
  Realtime            SSE
  SDK                 TypeScript package
  Logging             `log/slog`
  Testing             Go `testing`, `httptest`, PostgreSQL integration tests
  Deployment          Dashboard and API deployed independently as containers

The dashboard, API, and SDK live in one Git repository but remain
independently buildable and deployable. The Go backend is one deployable
application with internal modules, not a collection of microservices.

## 2. Repository layout

``` text
feature-flag-platform/
├── apps/
│   ├── dashboard/                 # Next.js
│   └── api/                       # Go modular monolith
│       ├── cmd/server/main.go
│       ├── internal/
│       │   ├── auth/
│       │   ├── project/
│       │   ├── environment/
│       │   ├── featureflag/
│       │   ├── config/
│       │   ├── stream/
│       │   ├── sdkkey/
│       │   ├── middleware/
│       │   └── database/
│       ├── migrations/
│       ├── sql/queries/
│       ├── sql/generated/
│       ├── go.mod
│       └── Dockerfile
├── packages/
│   └── sdk/                       # TypeScript SDK
├── docs/
│   ├── PRODUCT_SPEC.md
│   ├── TECHNICAL_SPEC.md
│   └── IMPLEMENTATION_PLAN.md
├── infrastructure/
│   └── docker/
├── .gitignore
└── README.md
```

Avoid adding Nx/Turborepo or a complex workspace tool solely for the
MVP. Use normal package scripts and a root Makefile initially.

## 3. Runtime components

### 3.1 Go API

Responsibilities: - Dashboard authentication and sessions. - Project and
environment CRUD. - Feature flag CRUD and state updates. - SDK
credential validation. - SDK configuration delivery. - SSE connection
lifecycle. - Configuration cache and version synchronization.

### 3.2 PostgreSQL

PostgreSQL is the durable source of truth. Supabase is used as a managed
PostgreSQL provider only. Do not make the business logic depend on
Supabase Auth, Edge Functions, Realtime, or proprietary database APIs.

### 3.3 In-memory configuration store

-   Store one immutable configuration snapshot per environment.
-   Read snapshots without querying PostgreSQL on each SDK request or
    evaluation.
-   Replace snapshots atomically after a committed update.
-   Use a concurrency-safe implementation (`atomic.Pointer` or an
    appropriately protected map).
-   On process startup, load active environment configurations from
    PostgreSQL.
-   If refresh fails after a committed write, mark the affected cache
    entry stale and retry/reload; do not publish a version that the
    cache cannot serve.
-   A snapshot is built from the data read and written inside the
    transaction that produced its version (§6.4), so the snapshot
    content and the version it reports always describe the same state.
    Snapshots are never rebuilt by re-querying PostgreSQL after commit.
-   The environment identifier is the snapshot key. The snapshot also
    carries the owning project ID and environment key, so a config
    response can be assembled without a second lookup.

### 3.4 SSE manager

-   Maintain subscribers by project/environment.
-   Each subscriber has a bounded event channel.
-   Publish version notifications, not full configuration payloads.
-   If a subscriber's channel is full, close that stream so the SDK
    reconnects and fetches the latest snapshot; never block the flag
    update path waiting for a slow client.
-   Send periodic heartbeat comments.
-   Respect request context cancellation and graceful shutdown.
-   Enforce connection limits and per-key rate limits appropriate to
    deployment capacity. Counters are held in memory; no external store
    is involved.
-   Key revocation closes that key's active streams immediately. On a
    single API instance the revocation handler signals the stream
    manager directly, so the revocation window for existing connections
    is effectively zero. Revalidation on the heartbeat tick bounds the
    window if that signal is missed (§8).
-   A revoked key's connection must be rejected at connect time, and
    the credential is revalidated on every config fetch.

## 4. Data model

Use UUID primary keys. Use `timestamptz` for timestamps and UTC in
application logic.

### 4.1 `users`

  Column          Type          Notes
  --------------- ------------- -----------------------
  id              uuid          PK
  email           text          Unique, normalized
  password_hash   text          Argon2id encoded hash
  created_at      timestamptz   Not null
  updated_at      timestamptz   Not null

MVP can bootstrap one administrator through a secure CLI/one-time setup
command. Do not add public self-registration.

### 4.2 `sessions`

  Column         Type          Notes
  -------------- ------------- ---------------------------------------
  id             uuid          PK
  user_id        uuid          FK users
  token_hash     text          Unique; never store raw session token
  expires_at     timestamptz   Not null
  created_at     timestamptz   Not null
  last_seen_at   timestamptz   Not null

Use a cryptographically random opaque session token in a Secure,
HttpOnly cookie. Store only its hash. Revoke by deleting the session
row.

**Session token hashing.** Session tokens are 32 bytes of CSPRNG
entropy. Their secrecy against an offline attacker rests entirely on
that entropy, not on a slow KDF. Hash session tokens with **SHA-256** and
look them up by hash. Do not use Argon2id here: it would be unnecessary
work on every authenticated request and provides no additional
protection against a high-entropy random token.

**Session lifetime.** `expires_at` is the absolute expiry.
`last_seen_at` records the most recent authenticated use and supports
idle expiry (an idle period longer than `SESSION_IDLE_TTL_HOURS`
revokes the session). Both values are required so expiry can be
evaluated without a background sweeper.

**Session cleanup.** Expired sessions are deleted lazily: on login the
handler deletes that user's expired sessions, and a bounded periodic
sweep removes expired rows in batches. No separate background service is
required.

### 4.3 `projects`

  Column       Type          Notes
  ------------ ------------- --------------------------
  id           uuid          PK
  owner_id     uuid          FK users, not null
  name         text          Not null
  slug         text          Unique stable identifier
  created_at   timestamptz   Not null
  updated_at   timestamptz   Not null

**Ownership.** `owner_id` makes the tenant boundary explicit and
enforceable in the schema rather than only in service code. Every
project-scoped authorization check resolves the acting administrator's
identity from the session and compares it against `owner_id`; a
client-supplied project ID is never trusted on its own. MVP provisions
all projects for the bootstrapped administrator, but the column and the
authorization check exist so that adding organizations later does not
require rewriting every project query.

### 4.4 `environments`

  Column           Type          Notes
  ---------------- ------------- ----------------------------------------
  id               uuid          PK
  project_id       uuid          FK projects
  key              text          `development`, `staging`, `production`
  name             text          Display name
  config_version   bigint        Starts at 1
  created_at       timestamptz   Not null
  updated_at       timestamptz   Not null

Unique constraint: `(project_id, key)`.

Also declare `UNIQUE (id, project_id)` so that composite foreign keys
from `feature_flag_states` and `sdk_keys` can enforce same-project
referencing (see §4.6 and §4.7).

Create the three default environments transactionally with project
creation.

**Environment key is immutable.** `key` is one of `development`,
`staging`, `production` and never changes. `name` is a mutable display
label only; editing it does not affect configuration identity and does
not increment `config_version`. MVP does not expose an environment
rename endpoint: the three canonical keys are sufficient, and custom
environment creation is deferred (see §16).

### 4.5 `feature_flags`

  Column        Type          Notes
  ------------- ------------- -----------------
  id            uuid          PK
  project_id    uuid          FK projects
  key           text          Stable flag key
  description   text          Nullable
  created_at    timestamptz   Not null
  updated_at    timestamptz   Not null

Unique constraint: `(project_id, key)`. Validate keys with a documented
conservative pattern such as lowercase letters, digits, underscore, and
hyphen.

Also declare `UNIQUE (id, project_id)` to support the composite foreign
keys described below.

### 4.6 `feature_flag_states`

  Column           Type          Notes
  ---------------- ------------- -------------------------
  id               uuid          PK
  flag_id          uuid          FK feature_flags
  environment_id   uuid          FK environments
  project_id       uuid          FK projects, not null
  enabled          boolean       Not null, default false
  updated_at       timestamptz   Not null

Unique constraint: `(flag_id, environment_id)`.

**Same-project integrity is enforced by the schema, not only by service
code.** A row may only reference a flag and an environment that belong
to the same project. This is enforced with composite foreign keys:

``` sql
FOREIGN KEY (flag_id, project_id)
  REFERENCES feature_flags (id, project_id) ON DELETE CASCADE,
FOREIGN KEY (environment_id, project_id)
  REFERENCES environments (id, project_id) ON DELETE CASCADE
```

This requires `UNIQUE (id, project_id)` on both `feature_flags` and
`environments`, as declared in §4.4 and §4.5. With these constraints a
cross-project state row is **not representable**, so the invariant holds
even if a handler forgets to check ownership. Service code still
performs the ownership check, because it must return a correct
authorization error rather than a constraint violation.

`project_id` is therefore denormalized deliberately: it is the join key
that makes the composite foreign keys expressible.

### 4.7 `sdk_keys`

  Column           Type          Notes
  ---------------- ------------- -------------------------
  id               uuid          PK
  project_id       uuid          FK projects
  environment_id   uuid          FK environments
  name             text          Human-readable label
  key_prefix       text          Safe display identifier
  key_hash         text          Unique hash of secret
  key_type         text          `client` or `server`
  revoked_at       timestamptz   Nullable
  created_at       timestamptz   Not null

Only the raw key is shown once at creation. Store a cryptographic hash;
never store the raw key. Both key types are read-only in the MVP. Client
keys are public identifiers with tightly scoped read-only access, not
secrets. Server keys are confidential and must never be included in
browser bundles.

**Same-project integrity.** `sdk_keys` repeats the composite foreign key
pattern from §4.6, because an SDK key whose `project_id` and
`environment_id` disagree would carry an inconsistent access scope:

``` sql
FOREIGN KEY (environment_id, project_id)
  REFERENCES environments (id, project_id) ON DELETE CASCADE
```

**Key format.** Keys are opaque strings with a type prefix so that the
key type is visible without a database lookup:

``` text
ff_client_<public-id>_<secret>
ff_server_<public-id>_<secret>
```

- `public-id` is 12 characters of base32 (Crockford, lowercase).
- `secret` is 32 bytes of CSPRNG entropy, base32-encoded.
- `key_prefix` stores the first 8 characters of the public ID, for safe
  display in the dashboard. It is never used for lookup or
  authentication.

The type prefix and the public ID are routing and display information.
Only the `secret` portion is the credential, and only its hash is
stored. See §4.10 for the hashing scheme per key type.

### 4.8 Indexes and constraints

-   Unique `users.email`.
-   Unique `sessions.token_hash`.
-   Unique `projects.slug`.
-   Unique `(id, owner_id)` on projects — supports ownership lookups.
-   Unique `(project_id, key)` on environments.
-   Unique `(id, project_id)` on environments — required by the composite
    foreign keys in §4.6 and §4.7.
-   Unique `(project_id, key)` on feature_flags.
-   Unique `(id, project_id)` on feature_flags — required by the
    composite foreign keys in §4.6.
-   Unique `(flag_id, environment_id)` on feature_flag_states.
-   Unique `sdk_keys.key_hash`.
-   Index `feature_flags.project_id`.
-   Index `environments.project_id`.
-   Index `feature_flag_states.environment_id` — snapshot build path.
-   Index `feature_flag_states.flag_id` — flag deletion path.
-   Index `feature_flag_states (project_id, environment_id)` — snapshot
    build path scoped by project.
-   Index `sessions.expires_at` — expiry sweep.
-   Index `sessions.user_id` — per-user session lookup and cleanup.
-   Index `sdk_keys.environment_id` for key listing by environment.
    Lookups by credential use the unique `sdk_keys.key_hash` index; the
    `environment_id` index serves listing, not authentication.

### 4.9 Delete semantics

Delete behavior is fixed at migration time, not decided during
implementation. MVP uses **hard delete** with explicit cascading, and
all deletions are performed inside a transaction.

-   Deleting a `project` cascades to `environments`, `feature_flags`,
    `feature_flag_states`, and `sdk_keys`.
-   Deleting a `feature_flag` cascades to its `feature_flag_states`.
-   Deleting an `environment` cascades to its `feature_flag_states` and
    `sdk_keys`.
-   Deleting a `user` cascades to `sessions` and restricts on
    `projects`, because a project must not silently lose its owner.

Flag deletion is a configuration change for every environment that held
a state for that flag, so it increments each affected environment's
`config_version` and publishes an event for each (see §6).

### 4.10 Credential hashing summary

Hashing primitives differ by credential type because the threat models
differ. Using one scheme for all of them is either unsafe (Argon2id on a
high-entropy token is wasted work on a hot path) or misleading.

  Credential               Scheme                           Notes
  ------------------------ -------------------------------- -------------------
  users.password_hash      Argon2id                         The only slow KDF
                                                            here. Calibrate
                                                            parameters for
                                                            deployment
                                                            hardware.
  sessions.token_hash      SHA-256                         High-entropy
                                                            random token;
                                                            lookup by hash.
  sdk_keys.key_hash        SHA-256, or HMAC-SHA-256 with a  A server-side
   (server type)           server-side pepper               pepper is optional
                                                            and held in
                                                            secret storage,
                                                            never in the
                                                            database.
  sdk_keys.key_hash        SHA-256                         Client keys are
   (client type)                                            public
                                                            identifiers; their
                                                            security comes
                                                            from scope
                                                            restriction, not
                                                            secrecy.

A pepper, when configured, is applied as
`HMAC-SHA-256(pepper, secret)`. Verification hashes the presented
secret with the same pepper and compares digests in constant time.

## 5. Configuration contract

SDK configuration response:

``` json
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

Rules: - Response contains only flag keys and Boolean values plus the
metadata required by the SDK. - No user data, password/session
information, SDK secret, or internal database metadata. - Configuration
is scoped to the credential's project and environment. - Missing flag
evaluation returns the caller-provided fallback, or `false` if the SDK
API defines that default. - A full snapshot is replaced atomically in
the SDK.

Additional contract details:

-   `flags` is always a JSON object. An environment with no flags
    returns `"flags": {}`, never `null` and never an array. This lets the
    SDK distinguish "flag absent" (use the caller's fallback) from "no
    flags exist".
-   `version` is serialized as a JSON number. `config_version` is a
    `bigint`, but JavaScript's `Number` is exact to 2^53 and an
    environment's version will never realistically approach that bound.
    The SDK compares versions as numbers, and the API must reject any
    request that would push `config_version` past 2^53.
-   Flag **descriptions are not part of the snapshot**. The snapshot
    contains only keys and Boolean values. Editing a description
    therefore does **not** change the configuration and does **not**
    increment `config_version` or publish an event.
-   The snapshot content and the `version` reported alongside it always
    describe the same configuration state. This invariant is maintained
    by the transaction and snapshot rules in §6 and must hold under
    concurrent writes.

## 6. Versioning and update consistency

`environments.config_version` is the version for that environment's
complete flag configuration.

### 6.1 Update transaction

For any effective configuration change: 1. Begin a PostgreSQL
transaction. 2. Lock the environment row with
`SELECT ... FROM environments WHERE id = $1 FOR UPDATE`. 3. Validate
that the flag and environment belong to the same project. 4. Read the
current state and compare it with the requested value. 5. If the value
is unchanged, roll back and return the existing version without
incrementing it. 6. Otherwise update the state and increment the
environment's `config_version` in the same transaction. 7. Build the
new snapshot **from the data read and written inside this transaction**.
8. Commit. 9. Atomically install the new snapshot into the in-memory
store. 10. Publish an SSE event containing the new version.

Do not emit an event before commit.

### 6.2 Lock order

**The environment row is locked first, always.** All configuration
changes within an environment serialize on that single row. Locking
`feature_flag_states` instead is insufficient: the version lives on
`environments`, so locking a different row would not serialize the
state update against the version increment, and the two could
interleave.

The lock order is fixed to prevent deadlock:

1. `environments` row (`FOR UPDATE`) — always first.
2. `feature_flag_states` rows — only after the environment lock is held.

Never acquire a project lock after an environment lock, and never
acquire two environment locks in the same transaction.

### 6.3 Idempotent writes and version increments

A write that requests the value the flag already holds is a **no-op**.
It does **not** increment `config_version` and does **not** publish an
SSE event.

This is a decided rule, not an open option. The `PUT` endpoint is
idempotent by design so that client retries are safe; a retry that
incremented the version would produce a different observable version for
the same logical state, trigger a spurious SSE event, and cause every
connected SDK to refetch an unchanged snapshot.

The mutation response still returns the current version so that a client
can confirm its intended state is in effect.

### 6.4 Snapshot construction

The snapshot published for version N is built from the values read and
written inside the transaction that produced version N. It is **not**
re-queried from PostgreSQL after the commit.

Re-querying after commit introduces a window in which a later committed
transaction has already advanced the database, so the rebuild could
produce content newer than the version being announced, or pair a
version with content that does not correspond to it. Building from
in-transaction data removes that window by construction and avoids an
extra database round trip.

The resulting snapshot and its version are consistent under concurrent
writes because the environment lock ensures that at most one
configuration change per environment is in flight.

### 6.5 Multi-environment changes

A flag change can affect more than one environment:

-   **Creating a flag** creates a default OFF state in every
    environment of the project. Each affected environment's
    `config_version` increments and each publishes its own SSE event.
-   **Deleting a flag** removes its state from every environment. Each
    affected environment's `config_version` increments and each
    publishes its own SSE event.
-   **Setting a flag's state** affects exactly one environment.
-   **Editing a description** affects no environment and increments no
    version, because descriptions are excluded from the snapshot (§5).

### 6.6 Version ordering

Within an environment, published versions are strictly increasing and
are published in commit order. The environment lock guarantees this.

### 6.7 Failure after commit

If cache installation fails after commit, the API must not publish a
false success event. Mark the cache entry stale, retry reload, and
expose operational health/metrics. The mutation response should
communicate that persistence succeeded but propagation is pending if
this exceptional state occurs.

The database remains the source of truth in this state. A subsequent
config fetch must not be served from a stale snapshot without
reflecting the degraded status in readiness.

## 7. REST API

All dashboard endpoints require an authenticated dashboard session,
except login and health endpoints.

### 7.1 Authentication

  Method   Path                    Purpose
  -------- ----------------------- ------------------------
  POST     `/api/v1/auth/login`    Sign in
  POST     `/api/v1/auth/logout`   Revoke current session
  GET      `/api/v1/auth/me`       Current administrator

Use cookie-based session auth for the first-party dashboard. Protect
state-changing cookie-authenticated requests against CSRF. Configure
CORS narrowly; prefer serving dashboard and API under a same-site domain
where practical.

**CSRF mechanism.** All state-changing dashboard requests
(`POST`, `PUT`, `PATCH`, `DELETE`) must carry a CSRF token that the
server validates against the session. The mechanism is a **double
submit token**: the server issues a token bound to the session, returns
it to the dashboard in a readable cookie and in the `GET
/api/v1/auth/me` response body, and requires it in the
`X-CSRF-Token` request header on every state-changing request. The
server compares the header against the value bound to the session and
rejects the request with `403` on mismatch or absence.

The token is bound to the session, so it is useless without a valid
session cookie and cannot be forged by a third-party site.

**Session cookie.** Name from `SESSION_COOKIE_NAME`. Flags: `HttpOnly`,
`Secure` when `COOKIE_SECURE=true` (required in production), `SameSite=Lax`,
and `Path=/`. The session cookie is scoped to the API's own paths so it
is not sent with unrelated dashboard asset requests.

### 7.2 Projects

  Method   Path                             Purpose
  -------- -------------------------------- -----------------------------------------
  GET      `/api/v1/projects`               List projects
  POST     `/api/v1/projects`               Create project and default environments
  GET      `/api/v1/projects/{projectID}`   Project detail
  PATCH    `/api/v1/projects/{projectID}`   Update project
  DELETE   `/api/v1/projects/{projectID}`   Delete project

### 7.3 Environments

  ---------------------------------------------------------------------------------------------
  Method                  Path                                          Purpose
  ----------------------- --------------------------------------------- -----------------------
  GET                     `/api/v1/projects/{projectID}/environments`   List environments

  ---------------------------------------------------------------------------------------------

### 7.4 Feature flags

  ------------------------------------------------------------------------------------------------------------------
  Method                  Path                                                               Purpose
  ----------------------- ------------------------------------------------------------------ -----------------------
  GET                     `/api/v1/projects/{projectID}/flags?environment={environmentID}`   List flags with
                                                                                             selected environment
                                                                                             state

  POST                    `/api/v1/projects/{projectID}/flags`                               Create flag and default
                                                                                             OFF states

  PATCH                   `/api/v1/flags/{flagID}`                                           Update description

  DELETE                  `/api/v1/flags/{flagID}`                                           Delete flag

  PUT                     `/api/v1/flags/{flagID}/environments/{environmentID}`              Set Boolean state
  ------------------------------------------------------------------------------------------------------------------

Use PUT for setting the desired Boolean value, not a blind toggle
operation. This makes retries idempotent.

Example request:

``` json
{
  "enabled": true
}
```

### 7.5 SDK endpoints

  Method   Path               Purpose
  -------- ------------------ -----------------------------------
  GET      `/sdk/v1/config`   Fetch full configuration snapshot
  GET      `/sdk/v1/stream`   Subscribe to SSE updates

Authenticate SDK endpoints with a scoped SDK key in an Authorization
header. The stream endpoint should support `fetch()` streaming in the
TypeScript SDK so custom authorization headers can be sent; do not put
long-lived keys in query strings.

**Authorization scheme.** SDK credentials are presented as:

``` text
Authorization: FF <sdk-key>
```

`FF` is a custom scheme, chosen over `Bearer` to avoid the implicit
OAuth semantics that `Bearer` carries and that intermediaries or future
middleware may mishandle. The server must accept only this scheme and
must not implement `Bearer` as an alias.

**SDK key issuance.** SDK keys are created and revoked through the
dashboard API. The creation request specifies the environment, a
human-readable name, and the key type:

``` json
{
  "environment_id": "uuid",
  "name": "web-frontend",
  "key_type": "client"
}
```

`key_type` is either `client` or `server`. The response contains the
raw key **once**; it is never retrievable again:

``` json
{
  "id": "uuid",
  "name": "web-frontend",
  "key_type": "client",
  "key_prefix": "a1b2c3d4",
  "key": "ff_client_a1b2c3d4e5f6_...",
  "created_at": "2026-01-01T00:00:00Z"
}
```

Revoking a key sets `revoked_at` and is permanent. See §10 for the
revocation window and §4.7 for the key format.

### 7.6 Operational endpoints

-   `GET /healthz`: process liveness.
-   `GET /readyz`: readiness, including database connectivity and cache
    initialization.

Use consistent JSON error responses with stable error codes. Do not leak
SQL errors or secrets.

## 8. SSE protocol

### Connection

-   `Content-Type: text/event-stream`
-   `Cache-Control: no-cache, no-transform`
-   `X-Accel-Buffering: no` where relevant to the reverse proxy. This
    header is nginx-specific; when a different ingress is used, apply
    that proxy's equivalent setting. Verify the effective behaviour in
    the deployed environment rather than assuming the header is honored.
-   Authenticate with the scoped SDK key using the `FF` scheme (§7.5).
-   Subscriber scope is derived from the validated key, never from
    untrusted project/environment query parameters.

### Events

Configuration notification:

``` text
id: 43
event: config.updated
data: {"version":43}
```

Heartbeat:

``` text
: heartbeat
```

The SDK treats the event as a signal to fetch the latest snapshot. It
should not assume every intermediate event is delivered.

**Event `id` is the environment's `config_version`.** The `id` field and
the `version` field in `data` carry the same number. This makes
`Last-Event-ID` interpretable as a version hint rather than an opaque
per-connection counter. The SSE `id` is a string on the wire; the SDK
parses it as a number.

### Reconnection

-   SDK reconnects with exponential backoff and jitter.
-   SDK tracks the last applied version.
-   On reconnect, SDK fetches the current configuration regardless of
    whether an event was missed.
-   If an event version is greater than the local version, fetch the
    snapshot.
-   Ignore duplicate or older versions.
-   Server may use `Last-Event-ID` as a hint, but the authoritative
    state is always the config endpoint.

**Snapshot fetch retries independently of event arrival.** If a snapshot
fetch fails, the SDK retries it with the same bounded backoff until it
succeeds. It must not wait for a subsequent SSE event to trigger another
attempt, because no further event may ever arrive — the SDK would
otherwise remain on a stale snapshot indefinitely while appearing
connected.

### Backpressure and lifecycle

-   Bounded per-client event channel.
-   Never block database mutation or other subscribers on a slow client.
-   Disconnect slow consumers and let them reconnect/fetch the latest
    snapshot.
-   Heartbeat to keep intermediaries from closing idle connections.
-   Close streams on context cancellation, key revocation detection,
    server shutdown, or configured maximum connection lifetime.
-   Validate proxy/load balancer idle timeout and buffering behavior in
    deployment.

### Operational constants

These are configurable, with the values below as the defaults. They are
listed here because the revocation window and proxy idle-timeout
behavior depend on them.

  Setting                          Default   Purpose
  -------------------------------- --------- ----------------------------------
  SSE_HEARTBEAT_INTERVAL_SECONDS   15        Heartbeat interval. Also the
                                               cadence at which a stream
                                               revalidates its credential.
  SSE_MAX_CONNECTION_SECONDS       3600      Maximum lifetime of one
                                               connection. The client
                                               reconnects; the snapshot
                                               fetch on reconnect
                                               guarantees no state is
                                               lost.
  SSE_SUBSCRIBER_QUEUE_SIZE        16        Bounded per-subscriber
                                               event queue. Overflow
                                               closes the stream.
  SSE_MAX_CONNECTIONS_PER_KEY      10        Concurrent connections per
                                               credential.

Connection limits and rate limits are enforced with **in-memory
per-credential counters** in the stream manager. No Redis or other
external store is involved.

**Revocation window.** On a single API instance, revoking a key
immediately closes that key's active SSE streams and rejects subsequent
config fetches. The revocation window is therefore effectively zero for
existing connections and immediate for new ones. Revalidation on the
heartbeat tick provides defense in depth, bounding the window at
`SSE_HEARTBEAT_INTERVAL_SECONDS` if the immediate signal is missed.

## 9. TypeScript SDK contract

Illustrative API:

``` ts
const client = createFeatureFlagClient({
  apiUrl: "https://api.example.com",
  sdkKey: "ff_client_...",
});

await client.initialize();

const enabled = client.isEnabled("new_dashboard", false);

const unsubscribe = client.on("change", () => {
  // Re-render or update application state.
});
```

Required behavior: - `initialize()` fetches the initial snapshot before
resolving. - `isEnabled(key, fallback)` reads only local memory. -
`on("change", callback)` notifies subscribers after an atomic snapshot
replacement. - SDK starts SSE after successful initialization. -
Reconnect with bounded exponential backoff and jitter. - Fetch latest
snapshot after reconnect. - Retain last valid snapshot on transient
errors. - Expose connection status for diagnostics, but no analytics or
user tracking. - `close()` stops streams and releases resources. - Do
not include server SDK secrets in browser applications.

**Transport requirements.**

-   The SDK must send credentials in the `Authorization: FF <sdk-key>`
    header (§7.5).
-   The SDK **must not use the browser `EventSource` API**. `EventSource`
    cannot set an `Authorization` header, so it cannot authenticate
    against the SDK endpoints. The SDK must read the SSE stream using
    `fetch()` with a streaming response body reader. This applies to
    browser builds as well as server-side builds, so there is one code
    path.
-   SDK keys are never placed in query strings, URLs, or log messages.

**Failure handling requirements.**

-   A failed snapshot fetch is retried with bounded backoff until it
    succeeds, independently of whether further SSE events arrive
    (§8).
-   The last valid snapshot is retained across all failures, including
    a failed reconnect, and continues to serve `isEnabled`.
-   A snapshot whose version is not newer than the currently applied
    version is discarded rather than applied.

Framework adapters (Angular/React) are not required for MVP; the core
TypeScript SDK should be framework-agnostic.

## 10. Authentication and security

### Dashboard

-   Bootstrap the initial administrator through a CLI/one-time secure
    setup path.
-   No public registration in MVP.
-   Hash passwords with Argon2id using parameters calibrated for
    deployment hardware and a maintained implementation.
-   Generate opaque session tokens using cryptographically secure
    randomness.
-   Store only token hashes in `sessions`, hashed with SHA-256 (§4.10).
-   Cookie flags: `HttpOnly`, `Secure` in production, `SameSite=Lax`,
    narrow `Path`, and appropriate expiry.
-   Rotate session on login; revoke on logout.
-   Add login rate limiting and generic authentication errors.
-   Protect cookie-authenticated mutations against CSRF using the
    double-submit mechanism (§7.1).
-   Never log passwords, raw session tokens, or raw SDK keys.

**Login rate limiting.** Login attempts are throttled with **in-memory**
counters, keyed on both the submitted email and the client IP address.
No Redis or other external store is involved. The key set is bounded:
counters are evicted after the window elapses, and the tracked key count
is capped so that an attacker cannot exhaust memory by submitting
distinct emails. The generic error message is returned whether the
address is unknown or the password is wrong.

### SDK

-   Client key: read-only, project/environment-scoped, assumed visible
    to browser users.
-   Server key: read-only, project/environment-scoped, confidential.
-   Hash keys at rest and show raw key only once. Hashing is per key
    type (§4.10).
-   Revocation must reject future config fetches and stream connections.
    On a single API instance, revocation **immediately** closes that
    key's active SSE streams. Revalidation on the heartbeat tick bounds
    the window at `SSE_HEARTBEAT_INTERVAL_SECONDS` (§8).
-   Apply rate limits and connection limits per credential/environment.

### Database

-   Use a dedicated runtime database role with only required privileges.
-   Use a separate migration role/connection for schema changes.
-   Keep Supabase database credentials server-side.
-   Use TLS for database connections.
-   Do not expose Supabase service-role keys to the dashboard or SDK.
-   Keep business logic portable to standard PostgreSQL.

## 11. Supabase PostgreSQL usage

Use Supabase as managed PostgreSQL only: - Go connects through `pgx`. -
Migrations run through a direct database connection or a suitable
session connection. - Long-running Go API should use a stable connection
strategy. Evaluate direct versus session pooler based on deployment
network reachability and connection limits. - Transaction pooler may be
suitable for some workloads, but verify driver/prepared-statement
compatibility before adopting it. - Configure a bounded `pgxpool` pool;
do not open a connection per request. - Set connection, query, and
transaction timeouts. - Monitor database connection count, CPU, memory,
storage, and query latency. - Confirm backup and point-in-time recovery
options for the selected Supabase plan.

Do not use Supabase Auth, Supabase Realtime, Edge Functions, or database
webhooks for the MVP.

## 12. Cache initialization and failure behavior

-   At startup, establish database connectivity and load configuration
    snapshots before marking the API ready.
-   SDK config reads use the in-memory snapshot.
-   Dashboard mutations persist to PostgreSQL first.
-   After commit, update the relevant snapshot and publish the new
    version.
-   On database outage, do not accept writes; serve the last valid
    cached SDK snapshot only if the service's availability policy allows
    it, and clearly expose degraded readiness.
-   On process restart, reload from PostgreSQL before accepting SDK
    config traffic.
-   No Redis is required for a single backend instance.

## 13. Scaling path

### MVP: single API instance

-   One Go API instance.
-   Supabase PostgreSQL.
-   In-memory snapshots and SSE subscriber manager.
-   SDK-side local cache.

### Growth: multiple API instances

The in-memory cache and SSE manager are instance-local. A write handled
by instance A will not automatically notify clients connected to
instance B.

Add cross-instance invalidation/broadcast only when deploying multiple
instances: - PostgreSQL `LISTEN/NOTIFY` can be a simple first step for
lightweight invalidation signals, with payloads containing
identifiers/version only. - Redis Pub/Sub is an option when operational
requirements justify it. - Each instance reloads the authoritative
snapshot from PostgreSQL after receiving an invalidation. - Consider a
durable outbox if guaranteed event processing becomes necessary.

Do not add Redis/Kafka/microservices pre-emptively.

## 14. Observability and operations

MVP minimum: - Structured JSON logs via `slog`. - Request ID and
correlation ID. - HTTP request count, latency, status codes. - Active
SSE connection count. - Config fetch count and latency. - Configuration
update and propagation latency. - Cache refresh failures. - Database
pool usage and query latency. - Liveness and readiness endpoints. -
Graceful shutdown for HTTP server and SSE streams.

Never log passwords, session tokens, raw SDK keys, or full authorization
headers.

## 15. Testing strategy

-   Unit tests for services, validation, auth, versioning, and cache
    replacement.
-   PostgreSQL integration tests for migrations, constraints,
    transactions, and project/environment isolation.
-   HTTP handler tests with `httptest`.
-   SSE tests for initial connection, notification, heartbeat,
    disconnect, reconnect, and slow consumer handling.
-   SDK tests for initialization, local evaluation, fallback, atomic
    updates, reconnect, and cleanup.
-   Race tests using `go test -race ./...`.
-   Load tests for config fetch and concurrent SSE connections before
    making capacity claims.
-   Security tests for credential scope, revoked keys, CSRF, login
    throttling, and secret leakage.

### 15.1 Test database strategy

Integration tests require a real PostgreSQL database. The strategy is
fixed so that the suite is reproducible and safe to run repeatedly.

-   Tests run against a **dedicated development/test database**, never
    against production.
-   The schema is prepared by applying the `golang-migrate` migrations
    at the start of the test run. Tests do not depend on a manually
    prepared database.
-   Each test that writes uses a **unique schema** created for that
    test and dropped afterwards, so tests are independent and can run
    concurrently without interfering with one another. `search_path` is
    set per connection.
-   The test harness connects with the migration connection string
    (`MIGRATION_DATABASE_URL`), because it needs schema-creation
    privileges. The application runtime role (`DATABASE_URL`) is
    deliberately **not** given those privileges, and a test asserts
    that.
-   The Supabase MCP server is configured `read_only=true` and cannot
    prepare the test schema. Migrations and test setup use a direct
    `golang-migrate` / `pgx` connection with normal database
    credentials.
-   Destructive operations in the test harness apply only to the
    per-test schema.

### 15.2 Required assertions

-   **Isolation is asserted at the database level.** A test attempts to
    insert a `feature_flag_states` row referencing a flag and an
    environment from different projects, directly against PostgreSQL,
    and asserts that the constraint rejects it. Service-layer tests
    alone are insufficient: they can pass while the schema still permits
    the invalid row.
-   **Version/content correspondence is asserted.** Beyond version
    monotonicity, a test asserts that the version reported by a config
    fetch always corresponds to the content in the same response. A
    snapshot must never report a version whose content is stale.
-   **Concurrent updates preserve both.** A test issues concurrent
    updates to the same environment and asserts that versions are
    strictly increasing, no increments are lost, and the final snapshot's
    version equals the number of effective changes applied.
-   **Snapshot reads are exercised concurrently.** A test reads
    snapshots from multiple goroutines while writers mutate them, so
    `go test -race` actually covers the shared-state surface.
-   **`isEnabled` performs no network I/O.** The SDK test replaces the
    transport with a counting stub and asserts the count is zero after
    repeated `isEnabled` calls. Asserting only the returned Boolean is
    not sufficient.
-   **Idempotent writes do not bump the version.** A test writes the
    same value twice and asserts the version is unchanged and no SSE
    event is published.
-   **SSE frames are parsed by a shared test helper** rather than by
    ad-hoc parsing in each test, to avoid duplicated and flaky frame
    parsing across chunk boundaries.

## 16. Open decisions to resolve during implementation

Resolved in this revision and no longer open:

-   ~~Whether deleting a project/flag is hard delete or soft delete in
    MVP.~~ Hard delete with explicit cascading (§4.9).
-   ~~Exact SDK key format and key rotation UX.~~ Key format is
    specified (§4.7); per-key-type hashing is specified (§4.10).
    Key *rotation* UX remains open.

Still open:

-   Exact Go and dependency versions.
-   Dashboard/API domain and cookie topology. The CSRF mechanism
    (§7.1) and cookie flags (§10) are specified, but the domain layout
    that determines the cookie's `SameSite` and `Secure` values in
    production is not.
-   Supabase plan, region, connection mode, backup policy, and
    deployment location.
-   Initial administrator bootstrap UX. The mechanism is a CLI/one-time
    command (§4.1), but the exact operator workflow is not.
-   SDK key rotation UX. The key format is specified (§4.7); whether
    rotation issues a new key or reuses the existing one, and how
    overlap during rotation is handled, is not.
-   Numeric capacity targets for concurrent SSE connections and
    configuration propagation latency.

These should be decided before production launch, but do not block the
initial skeleton and core CRUD work.

### 16.1 Decisions required before M1 schema work

The following are **not** deferred. They change the schema or the
transaction protocol, so they must be settled before the first migration
is written.

-   ~~Delete semantics.~~ Fixed in §4.9.
-   ~~Credential hashing per type.~~ Fixed in §4.10.
-   ~~Project ownership column.~~ `projects.owner_id` added (§4.3).
-   ~~Same-project integrity for flag states and SDK keys.~~ Composite
    foreign keys fixed (§4.6, §4.7).
-   ~~Version increment on idempotent writes.~~ Fixed in §6.3.
-   ~~Lock order and snapshot construction.~~ Fixed in §6.2 and §6.4.
-   ~~Test database strategy.~~ Fixed in §15.1.
