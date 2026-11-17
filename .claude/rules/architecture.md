# Architecture Rules

- Keep dashboard, API, and SDK independently buildable in the monorepo.
- Keep the Go API a modular monolith.
- Use explicit module boundaries: auth, project, environment, featureflag, config, stream, sdkkey, database.
- Avoid generic abstractions that have only one implementation and no demonstrated need.
- Keep domain/service logic independent of HTTP and SQL transport.
- Do not add Redis, Kafka, WebSockets, microservices, Kubernetes-specific dependencies, or distributed caches to MVP without an approved requirement.
- Supabase is PostgreSQL hosting only; preserve portability to standard PostgreSQL.
- Update `docs/TECHNICAL_SPEC.md` when an approved architecture decision changes.
