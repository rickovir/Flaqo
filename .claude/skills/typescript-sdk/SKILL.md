---
name: typescript-sdk
description: Use when implementing or reviewing the framework-agnostic TypeScript feature-flag SDK, its public API, local evaluation, snapshot updates, SSE reconnect, or package build.
---

# TypeScript SDK Workflow

1. Read the SDK contract in `docs/TECHNICAL_SPEC.md` (§9) and product acceptance criteria.
2. Keep the core SDK framework-agnostic; do not add Angular/React adapters to MVP unless explicitly requested.
3. `initialize()` fetches the initial config before resolving.
4. `isEnabled(key, fallback)` reads only the in-memory snapshot; never performs network I/O.
5. Apply snapshots atomically and notify listeners only after the new snapshot is active.
6. SSE events are version notifications; fetch the latest snapshot after a newer event.
7. Reconnect with bounded exponential backoff and jitter; fetch current config after reconnect to recover missed events.
8. Retry a failed snapshot fetch independently of event arrival. Never wait for the next SSE event to retry, since none may arrive.
9. Retain the last valid snapshot during transient failures.
10. Send credentials as `Authorization: FF <sdk-key>`. Never put keys in query strings or URLs.
11. Never use the browser `EventSource` API; it cannot send an `Authorization` header. Read the SSE stream with `fetch()` and a streaming body reader.
12. Provide `close()` to stop streaming and release listeners/resources.
13. Do not expose server SDK credentials in browser builds.
14. Test initialization, fallback, local reads, update notifications, reconnect, missed events, cleanup, and malformed responses.
15. Assert that `isEnabled` triggers zero transport calls, using a counting stub rather than only checking the returned value.
