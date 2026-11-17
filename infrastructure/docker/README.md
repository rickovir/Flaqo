# Running the platform with Docker

This directory holds the container setup for local development. The
database is not containerised: Supabase is the managed PostgreSQL
provider.

## Prerequisites

- Docker Desktop, with the Linux containers engine
- The migrations applied to the database
- A `.env` file with the connection strings and secrets

## One-time setup

1. Copy the repository `.env.example` to `.env` in the repository root
   and fill in the values. See the database section below for the
   connection strings.

2. Apply the migrations using the migration role:

   ```bash
   make migrate-up
   ```

3. Create the first administrator:

   ```bash
   cd apps/api
   BOOTSTRAP_ADMIN_EMAIL=admin@example.com \
   BOOTSTRAP_ADMIN_PASSWORD='a-strong-password' \
   go run ./cmd/admin bootstrap
   ```

   Remove the password from the environment afterwards.

4. Copy `.env` into this directory so Compose can read it:

   ```bash
   cp ../../.env .env
   ```

   Compose reads `.env` from the directory that holds the compose file.
   The `.env` files are gitignored.

## Running

```bash
cd infrastructure/docker
docker compose up --build -d
docker compose ps
```

The services are then available at:

| Service | URL | Purpose |
| ------- | --- | ------- |
| Dashboard | http://localhost:3000 | The admin interface |
| API | http://localhost:8080 | The REST and SSE API |
| Demo application | http://localhost:4000 | Shows the SDK in use |

The demo application needs a real SDK key. Issue one from the
dashboard, then add it to the `.env` file:

```dotenv
DEMO_SDK_KEY=ff_client_...
```

and restart that service:

```bash
docker compose up -d demo-app
```

To stop the stack:

```bash
docker compose down
```

## The demo application

It shows the SDK's behaviour: the flag values it holds, and the effect
of a change made in the dashboard. It is not a product feature.

The server holds the SDK's in-memory snapshot, so evaluating a flag never
performs a network request. The SSE stream only signals that the
configuration changed; the SDK then fetches the new snapshot and
replaces the old one atomically.

## Database connectivity from containers

The Supabase direct host, `db.<project-ref>.supabase.co`, resolves to an
IPv6 address only. Docker's default bridge network has no IPv6 route, so
a container cannot reach it.

The compose file therefore creates its network with IPv6 enabled:

``` yaml
networks:
  featureflag:
    driver: bridge
    enable_ipv6: true
    ipam:
      config:
        - subnet: 172.31.0.0/16
        - subnet: fd00:ff::/64
```

With that network, containers reach the database through the direct host
and the connection strings need no special handling.

If a container reports `network is unreachable` for the database host,
confirm that the network was created with IPv6:

```bash
docker network inspect feature-flag-platform_featureflag \
  --format '{{.EnableIPv6}}'
```

It must print `true`.

The alternative is the regional pooler, which has IPv4 addresses. It
needs the tenant identifier from the Supabase dashboard, and the
username takes the form `<role>.<tenant>`. The pooler is the right
choice for a deployment host without IPv6.

## Production

The compose file is for local development. It does not configure TLS, a
reverse proxy, or secret management.

For a deployment:

- Build the images and push them to a registry. The API image is
  `apps/api/Dockerfile` and the dashboard image is
  `apps/dashboard/Dockerfile`.
- Provide the configuration through the deployment's secret store. The
  API reads it from the environment.
- Set `COOKIE_SECURE=true` so the session cookie is only sent over
  HTTPS.
- Put the dashboard and the API behind a reverse proxy on one domain, so
  the browser sees a single origin. A cross-origin setup needs
  `ALLOWED_DASHBOARD_ORIGINS` set to the dashboard's origin, and the
  session cookie's `SameSite` policy must permit it.
- The proxy must not buffer the SSE response. For nginx, that means
  `proxy_buffering off` on the `/sdk/v1/stream` location, and the API
  already sends `X-Accel-Buffering: no`.
- The proxy's idle timeout must exceed the SSE heartbeat interval, or
  connections are closed between heartbeats.
- Run the API as a single instance. The configuration cache and the SSE
  subscriber registry are in process, so a second instance would not see
  changes made through the first.

## Applying migrations in a deployment

Migrations run separately from the API, using the migration role:

```bash
cd apps/api
MIGRATION_DATABASE_URL='...' go run github.com/golang-migrate/migrate/v4/cmd/migrate \
  -path migrations -database "$MIGRATION_DATABASE_URL" up
```

The API's runtime role cannot modify the schema, so it cannot apply
migrations itself. That separation is deliberate.
