---
paths:
  - "apps/api/**"
  - "apps/dashboard/**"
  - "packages/sdk/**"
---

# Security Rules

- Dashboard auth is custom opaque server-side sessions, not Supabase Auth.
- Passwords use Argon2id with maintained implementation and calibrated parameters. Argon2id is used for `users.password_hash` only.
- Session tokens and SDK key secrets use SHA-256 (optionally HMAC-SHA-256 with a server-side pepper for server SDK keys). Do not use Argon2id on high-entropy random tokens or on the SDK key lookup hot path.
- Generate session and SDK secrets using cryptographically secure randomness.
- Store only hashes of raw session tokens and SDK keys.
- Set production session cookies `HttpOnly`, `Secure`, and an appropriate `SameSite` policy.
- Protect cookie-authenticated state-changing requests against CSRF using the session-bound double-submit token in the `X-CSRF-Token` header.
- SDK endpoints authenticate with `Authorization: FF <sdk-key>`. Do not accept `Bearer` as an alias.
- Rate-limit login and credential-sensitive endpoints. Login throttling counters are in-memory, bounded, and keyed on email plus client IP. Do not introduce Redis for this.
- Never return raw SDK key after initial creation; show it once.
- Client SDK keys are public and read-only; server SDK keys are confidential and read-only.
- Derive project/environment scope from validated credentials, not arbitrary client parameters.
- Revoke keys and terminate/reject their active access within the documented revocation window. On a single instance, revocation closes active SSE streams immediately.
- Never log passwords, cookies, Authorization headers, session tokens, or raw SDK keys.
- Do not put secrets in Next.js `NEXT_PUBLIC_*` variables.
- Check CORS, origin validation, error messages, and dependency vulnerabilities.
- Never commit secrets. `.env`, `.claude/settings.local.json`, and any file containing a live token must be gitignored. Tracked config files use placeholders or environment references only.
