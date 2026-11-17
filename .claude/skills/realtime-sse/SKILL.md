---
name: realtime-sse
description: Use when implementing or reviewing SSE, configuration cache synchronization, version notifications, stream lifecycle, reconnect, backpressure, or concurrent update propagation.
---

# Realtime SSE Workflow

1. Read the SSE and configuration consistency sections of `docs/TECHNICAL_SPEC.md` (§6 and §8).
2. SSE is one-way notification delivery; do not introduce WebSockets.
3. Persist the desired state and increment the environment version transactionally.
4. Lock the `environments` row with `FOR UPDATE` first, before any `feature_flag_states` read or write. This lock order is fixed.
5. A write that requests the value already stored is a no-op: no version increment, no event.
6. Build the published snapshot from data read and written inside the transaction. Never re-query PostgreSQL after commit to rebuild a snapshot.
7. Publish only after commit and after the backend can serve the corresponding committed snapshot.
8. Use immutable snapshots and concurrency-safe atomic replacement.
9. Event payload contains the version only; SDK fetches the full authoritative snapshot. The SSE event `id` equals the environment's `config_version`.
10. Use bounded per-subscriber queues. Never let a slow client block writes or other subscribers.
11. Disconnect slow consumers so they can reconnect and fetch the latest snapshot.
12. Handle heartbeats, request cancellation, graceful shutdown, key revocation, and proxy buffering/timeouts.
13. Revoking a key closes its active streams immediately on a single instance; revalidation on the heartbeat tick bounds the window.
14. Reconnect must recover missed events by fetching the current snapshot; do not rely on every event being delivered.
15. Test ordering, duplicate events, missed events, concurrent toggles, slow consumers, and race conditions.
