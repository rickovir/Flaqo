---
paths:
  - "**/*_test.go"
  - "apps/dashboard/**"
  - "packages/sdk/**"
---

# Testing Rules

- Add tests for every behavior change.
- Use PostgreSQL integration tests for constraints, transactions, and isolation; mocks alone are insufficient for database guarantees.
- Test project and environment isolation explicitly.
- Test configuration version monotonicity and concurrent updates.
- Test SSE authentication, heartbeat, disconnect, backpressure, reconnect, and missed-event recovery.
- Test SDK local evaluation, fallback, atomic snapshot replacement, reconnect, and close/cleanup.
- Run Go race tests for concurrency changes.
- Do not weaken or delete a test merely to make a change pass; explain and justify any test behavior change.
- Report exact commands and outcomes. Do not claim unrun checks passed.
