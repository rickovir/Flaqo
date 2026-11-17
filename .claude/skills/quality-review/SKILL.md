---
name: quality-review
description: Use before declaring a feature, bug fix, or milestone complete; review correctness, security, tests, architecture, and documentation against the MVP.
---

# Quality Review Workflow

Review the diff, not just the implementation summary.

## Review dimensions
- Product scope: Boolean-only MVP; no unapproved feature expansion.
- Correctness: edge cases, idempotency, transaction boundaries, error handling.
- Isolation: project/environment authorization and data boundaries.
- Concurrency: races, goroutine leaks, shared mutable state, bounded queues.
- Realtime: commit-before-publish, version monotonicity, reconnect and missed events.
- Security: secret handling, session lifecycle, CSRF, rate limits, key revocation.
- Database: constraints, migration safety, indexes justified by queries.
- SDK: local-only evaluation, atomic snapshots, cleanup, fallback.
- Tests: meaningful positive and negative cases; race tests where relevant.
- Operations: logs, timeouts, health checks, configuration and deployment impact.

## Output
Classify findings as Critical, High, Medium, or Low. Include file/line references, impact, reproduction or reasoning, and a concrete fix. Distinguish confirmed defects from questions or hardening suggestions. Do not modify code during a review unless asked.
