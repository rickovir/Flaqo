---
paths:
  - "apps/api/migrations/**"
  - "apps/api/sql/**"
  - "apps/api/internal/database/**"
---

# PostgreSQL Rules

- PostgreSQL is the source of truth; Supabase is only the managed provider.
- Use migrations for every schema change. Never modify a deployed schema manually as the only record of a change.
- Use `timestamptz` and UTC semantics.
- Add constraints for uniqueness and referential integrity; validate authorization and cross-project ownership in service logic too.
- Use parameterized SQL only. Never concatenate untrusted values into SQL.
- Use transactions for multi-row operations and configuration version updates.
- A flag key is unique within a project.
- A flag/environment state pair is unique.
- A flag and environment must belong to the same project. Enforce this with composite foreign keys in the schema, not only in service logic. `feature_flag_states` and `sdk_keys` carry `project_id` and reference `feature_flags (id, project_id)` and `environments (id, project_id)`.
- Lock the `environments` row with `FOR UPDATE` first, before any `feature_flag_states` read or write. Never change this lock order.
- Configuration version increments must be serialized per environment and committed atomically with the state change.
- A write that requests the value already stored is a no-op: it must not increment `config_version` and must not publish an event.
- Build the published snapshot from data read and written inside the transaction. Do not re-query PostgreSQL after commit to rebuild a snapshot.
- Use a bounded `pgxpool`; never open a database connection per request.
- Separate runtime and migration credentials/roles where practical. The runtime role must not have schema-modification privileges.
- Never run destructive migrations, database resets, or production SQL without explicit approval.
- Review indexes against actual query patterns; do not add speculative indexes.
