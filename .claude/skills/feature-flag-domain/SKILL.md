---
name: feature-flag-domain
description: Use when implementing or reviewing project, environment, Boolean feature flag, configuration version, SDK configuration, or feature-flag business rules.
---

# Feature Flag Domain Workflow

1. Read `docs/PRODUCT_SPEC.md` and relevant parts of `docs/TECHNICAL_SPEC.md`.
2. Preserve MVP boundaries:
   - Boolean values only.
   - No experiments, targeting, segments, analytics, percentage rollout, or JSON flags.
   - A flag belongs to one project.
   - Its state is independent per environment.
   - New environment states default to OFF.
3. Validate project/environment ownership server-side.
4. Use desired-state updates (`enabled: true|false`) so retries are idempotent.
5. Increment the environment configuration version only for an effective configuration change. A write that requests the value already stored is a no-op: no increment, no event.
6. Flag creation and deletion affect every environment of the project. Increment each affected environment's version and publish an event per environment. A description edit affects no environment and increments no version, because descriptions are excluded from the snapshot.
7. Keep the config response minimal: project/environment identity, version, and flag-key-to-Boolean map. `flags` is always an object, never `null`.
8. Add tests for default OFF, duplicate keys, environment isolation, unknown flag fallback, and version changes.
9. Check that no user attributes or personal data are introduced.
10. Summarize any proposed scope expansion and ask for approval before implementing it.
