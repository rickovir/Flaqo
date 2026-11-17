---
name: postgres-database
description: Use when designing or changing PostgreSQL schema, migrations, SQL queries, indexes, transactions, Supabase database connections, or database performance.
---

# PostgreSQL Database Workflow

1. Read the database section of `docs/TECHNICAL_SPEC.md` (§4) and `.claude/rules/database.md`.
2. Confirm current schema and migration state before proposing changes.
3. Prefer standard PostgreSQL features to preserve portability away from Supabase.
4. Write a forward migration and consider rollback/recovery implications.
5. Add constraints for invariants; do not rely only on application validation. Same-project flag/environment pairing is enforced with composite foreign keys.
6. Use transactions for multi-step writes.
7. For flag changes, lock the `environments` row with `FOR UPDATE` first, then update the state and increment `config_version` in the same transaction. Build the snapshot from in-transaction data.
8. Review query plans and indexes against real query shapes when performance is in scope.
9. Keep runtime database permissions minimal and separate from migration permissions.
10. Never reset, drop, truncate, or apply destructive changes to a shared/production database without explicit approval.
11. Run migrations and integration tests only against the explicitly selected development/test database, following the test database strategy in `docs/TECHNICAL_SPEC.md` §15.1. The Supabase MCP server is read-only and cannot prepare the test schema.
