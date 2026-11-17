---
name: authentication-security
description: Use when implementing or reviewing dashboard login, password hashing, sessions, cookies, CSRF, SDK credentials, authorization, secrets, or security-sensitive code.
---

# Authentication and Security Workflow

1. Read the security section of `docs/TECHNICAL_SPEC.md` (§10), the credential hashing rules (§4.10), and `.claude/rules/security.md`.
2. Dashboard auth uses custom opaque server-side sessions.
3. Hash passwords with Argon2id. Hash session tokens and SDK key secrets with SHA-256, optionally HMAC-SHA-256 with a server-side pepper for server SDK keys. Argon2id on a high-entropy token is wasted work on the hot path.
4. Use cryptographically secure randomness for session tokens and SDK secrets.
5. Store only hashes of session tokens and SDK keys.
6. Use Secure/HttpOnly/SameSite cookies in production and protect cookie-authenticated mutations against the session-bound double-submit CSRF token in the `X-CSRF-Token` header.
7. Apply login throttling with bounded in-memory counters keyed on email plus client IP, and return generic authentication failures.
8. SDK endpoints authenticate with `Authorization: FF <sdk-key>`; do not accept `Bearer` as an alias.
9. SDK keys are read-only and scoped to exactly one project/environment.
10. Treat browser client keys as public identifiers; never embed server keys in browser bundles.
11. Derive access scope from the authenticated credential; reject cross-project/environment access.
12. Avoid logging secrets or returning them in errors.
13. Add negative tests: invalid session, expired/revoked session, revoked SDK key, wrong scope, CSRF failure, brute-force throttling.
14. Do not claim a security review is complete without inspecting code paths and test evidence.
