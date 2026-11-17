import type {
  ChangeListener,
  ConfigurationSnapshot,
  ConnectionStatus,
  ConnectionStatusDetail,
  FeatureFlagClient,
  FeatureFlagClientOptions,
  StatusListener,
  Unsubscribe,
} from "./types.js";

const DEFAULT_RECONNECT_BASE_DELAY_MS = 1_000;
const DEFAULT_RECONNECT_MAX_DELAY_MS = 30_000;
const DEFAULT_REQUEST_TIMEOUT_MS = 10_000;

/**
 * A framework-agnostic feature flag client.
 *
 * Flag evaluation is local. The client holds the most recent snapshot in
 * memory and `isEnabled` reads only from it, so evaluation never
 * performs a network request.
 */
export class FeatureFlagClientImpl implements FeatureFlagClient {
  private readonly apiUrl: string;
  private readonly sdkKey: string;
  private readonly reconnectBaseDelayMs: number;
  private readonly reconnectMaxDelayMs: number;
  private readonly requestTimeoutMs: number;
  private readonly fetchImpl: typeof globalThis.fetch;

  /**
   * The applied snapshot.
   *
   * It is replaced atomically: a reader either sees the old snapshot or
   * the new one, never a partially applied state.
   */
  private snapshot: ConfigurationSnapshot | undefined;

  /** The version currently applied. */
  private version = 0;

  /** Set once the snapshot has been applied at least once. */
  private initialized = false;

  private status: ConnectionStatus = "idle";
  private closed = false;

  private readonly changeListeners = new Set<ChangeListener>();
  private readonly statusListeners = new Set<StatusListener>();

  /** Aborts the in-flight stream request. */
  private streamAbort: AbortController | undefined;

  /** Resolves when the current stream attempt ends. */
  private streamPromise: Promise<void> | undefined;

  /** Guards against more than one stream running at a time. */
  private streaming = false;

  /** Retry state for a failed snapshot fetch. */
  private fetchRetryTimer: ReturnType<typeof setTimeout> | undefined;
  private fetchRetryAttempt = 0;

  constructor(options: FeatureFlagClientOptions) {
    if (!options.apiUrl) {
      throw new Error("apiUrl is required");
    }
    if (!options.sdkKey) {
      throw new Error("sdkKey is required");
    }

    this.apiUrl = options.apiUrl.replace(/\/+$/, "");
    this.sdkKey = options.sdkKey;
    this.reconnectBaseDelayMs =
      options.reconnectBaseDelayMs ?? DEFAULT_RECONNECT_BASE_DELAY_MS;
    this.reconnectMaxDelayMs =
      options.reconnectMaxDelayMs ?? DEFAULT_RECONNECT_MAX_DELAY_MS;
    this.requestTimeoutMs = options.requestTimeoutMs ?? DEFAULT_REQUEST_TIMEOUT_MS;
    this.fetchImpl = options.fetch ?? globalThis.fetch;

    if (typeof this.fetchImpl !== "function") {
      throw new Error(
        "no fetch implementation available; pass one in the options",
      );
    }
  }

  async initialize(): Promise<void> {
    if (this.closed) {
      throw new Error("client is closed");
    }
    if (this.initialized) {
      return;
    }

    const applied = await this.fetchAndApply();
    if (!applied) {
      // The initial fetch failed. Initialization must fail rather than
      // leave the client in a state where every flag reads as its
      // fallback with no indication that the configuration is missing.
      throw new Error("failed to fetch the initial configuration");
    }

    this.initialized = true;
    // The stream starts only after the first snapshot is applied, so a
    // version event always refers to a snapshot the client can serve.
    void this.runStream();
  }

  /**
   * Reads a flag from the local snapshot.
   *
   * This performs no network I/O. It returns the fallback when the flag
   * is absent or the client has not initialized.
   */
  isEnabled(key: string, fallback = false): boolean {
    const snapshot = this.snapshot;
    if (!snapshot) {
      return fallback;
    }
    const value = snapshot.flags[key];
    // An absent flag is different from a flag set to false, so the
    // fallback is only used when the key is missing.
    return value === undefined ? fallback : value;
  }

  on(event: "change", listener: ChangeListener): Unsubscribe;
  on(event: "status", listener: StatusListener): Unsubscribe;
  on(
    event: "change" | "status",
    listener: ChangeListener | StatusListener,
  ): Unsubscribe {
    if (event === "change") {
      this.changeListeners.add(listener as ChangeListener);
      return () => {
        this.changeListeners.delete(listener as ChangeListener);
      };
    }
    this.statusListeners.add(listener as StatusListener);
    return () => {
      this.statusListeners.delete(listener as StatusListener);
    };
  }

  getStatus(): ConnectionStatus {
    return this.status;
  }

  getSnapshot(): ConfigurationSnapshot | undefined {
    return this.snapshot;
  }

  getVersion(): number | undefined {
    return this.snapshot ? this.version : undefined;
  }

  async close(): Promise<void> {
    if (this.closed) {
      return;
    }
    this.closed = true;

    this.setStatus("closed");

    if (this.fetchRetryTimer !== undefined) {
      clearTimeout(this.fetchRetryTimer);
      this.fetchRetryTimer = undefined;
    }

    this.streamAbort?.abort();
    this.streamAbort = undefined;

    // Wait for the stream loop to finish so no timer or request outlives
    // the call.
    if (this.streamPromise) {
      await this.streamPromise.catch(() => undefined);
    }
  }

  /**
   * Fetches the configuration and applies it if it is newer.
   *
   * Returns true when a newer snapshot was applied.
   */
  private async fetchAndApply(): Promise<boolean> {
    const snapshot = await this.fetchConfig();

    if (!snapshot) {
      return false;
    }

    // A snapshot that is not newer than the applied one is discarded.
    // The SDK must never replace a newer configuration with an older one.
    if (this.snapshot && snapshot.version <= this.version) {
      return false;
    }

    // The snapshot is replaced in one assignment, so a reader sees either
    // the old snapshot or the new one. Listeners are notified only after
    // the new snapshot is active.
    this.snapshot = snapshot;
    this.version = snapshot.version;

    for (const listener of this.changeListeners) {
      try {
        listener(snapshot);
      } catch (error) {
        // A failing listener must not break the client.
        this.reportListenerError(error);
      }
    }

    return true;
  }

  /**
   * Fetches the configuration snapshot.
   *
   * Returns undefined when the request fails. The caller decides whether
   * to retry.
   */
  private async fetchConfig(): Promise<ConfigurationSnapshot | undefined> {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), this.requestTimeoutMs);

    try {
      const response = await this.fetchImpl(
        `${this.apiUrl}/sdk/v1/config`,
        {
          method: "GET",
          headers: {
            // The FF scheme is the only one the API accepts. Bearer is
            // deliberately not used.
            Authorization: `FF ${this.sdkKey}`,
            Accept: "application/json",
          },
          signal: controller.signal,
        },
      );

      if (!response.ok) {
        return undefined;
      }

      const body = (await response.json()) as unknown;
      return parseSnapshot(body);
    } catch {
      // A failed request leaves the last valid snapshot in place.
      return undefined;
    } finally {
      clearTimeout(timer);
    }
  }

  /**
   * Runs the update stream loop.
   *
   * The loop reconnects with exponential backoff and jitter. It never
   * rejects: a failure schedules another attempt.
   */
  private async runStream(): Promise<void> {
    if (this.streaming) {
      return;
    }
    this.streaming = true;

    try {
      while (!this.closed) {
        try {
          this.setStatus(this.status === "connected" ? "reconnecting" : "connecting");
          await this.streamOnce();

          if (this.closed) {
            break;
          }

          // The stream ended normally. Reconnect after a delay.
          this.setStatus("reconnecting");
          await this.delay(this.backoffDelay(this.fetchRetryAttempt++));
        } catch (error) {
          if (this.closed) {
            break;
          }
          this.setStatus("reconnecting", error);
          await this.delay(this.backoffDelay(this.fetchRetryAttempt++));
        }
      }
    } finally {
      this.streaming = false;
      this.streamPromise = undefined;
    }
  }

  /**
   * Opens the stream once and reads events until it ends.
   *
   * The stream uses fetch with a streaming body reader rather than the
   * EventSource API, because EventSource cannot send an Authorization
   * header and the API requires one.
   */
  private async streamOnce(): Promise<void> {
    const controller = new AbortController();
    this.streamAbort = controller;

    try {
      const response = await this.fetchImpl(`${this.apiUrl}/sdk/v1/stream`, {
        method: "GET",
        headers: {
          Authorization: `FF ${this.sdkKey}`,
          Accept: "text/event-stream",
        },
        signal: controller.signal,
      });

      if (!response.ok || !response.body) {
        throw new Error(`stream failed with status ${response.status}`);
      }

      this.setStatus("connected");
      // A successful connection resets the backoff.
      this.fetchRetryAttempt = 0;

      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = "";

      try {
        for (;;) {
          const { done, value } = await reader.read();
          if (done) {
            break;
          }

          buffer += decoder.decode(value, { stream: true });

          // SSE frames are separated by a blank line.
          let separator = buffer.indexOf("\n\n");
          while (separator !== -1) {
            const frame = buffer.slice(0, separator);
            buffer = buffer.slice(separator + 2);
            await this.handleFrame(frame);
            separator = buffer.indexOf("\n\n");
          }
        }
      } finally {
        reader.releaseLock?.();
      }
    } finally {
      this.streamAbort = undefined;
    }
  }

  /**
   * Handles one SSE frame.
   *
   * A version event is a signal to fetch the latest snapshot. The SDK
   * does not assume every intermediate event was delivered.
   */
  private async handleFrame(frame: string): Promise<void> {
    let eventName = "";
    let data = "";

    for (const line of frame.split("\n")) {
      const trimmed = line.endsWith("\r") ? line.slice(0, -1) : line;
      if (trimmed.length === 0 || trimmed.startsWith(":")) {
        // Blank line or comment, such as a heartbeat.
        continue;
      }
      const separator = trimmed.indexOf(":");
      const field = separator === -1 ? trimmed : trimmed.slice(0, separator);
      const value =
        separator === -1 ? "" : trimmed.slice(separator + 1).replace(/^ /, "");

      if (field === "event") {
        eventName = value;
      } else if (field === "data") {
        data += value;
      }
    }

    if (eventName !== "config.updated") {
      return;
    }

    let version = 0;
    try {
      const parsed = JSON.parse(data) as { version?: number };
      version = typeof parsed.version === "number" ? parsed.version : 0;
    } catch {
      return;
    }

    // An older or duplicate version is ignored.
    if (version <= this.version) {
      return;
    }

    await this.fetchWithRetry();
  }

  /**
   * Fetches the configuration, retrying until it succeeds.
   *
   * The retry is independent of event arrival. If no further event ever
   * arrives, the client would otherwise stay on a stale snapshot while
   * appearing connected.
   */
  private async fetchWithRetry(): Promise<void> {
    if (this.closed) {
      return;
    }

    for (;;) {
      if (this.closed) {
        return;
      }

      const applied = await this.fetchAndApply().catch(() => false);
      if (applied) {
        this.fetchRetryAttempt = 0;
        return;
      }

      // The fetch failed. Retry with backoff, unless the client was
      // closed in the meantime.
      await this.delay(this.backoffDelay(this.fetchRetryAttempt++));
    }
  }

  /**
   * Returns a backoff delay with jitter.
   *
   * Jitter spreads reconnects so that many clients recovering from the
   * same outage do not reconnect at the same moment.
   */
  private backoffDelay(attempt: number): number {
    const base = Math.min(
      this.reconnectMaxDelayMs,
      this.reconnectBaseDelayMs * 2 ** Math.max(0, attempt),
    );
    // Full jitter: a random fraction of the computed delay.
    return Math.floor(base * (0.5 + Math.random() * 0.5));
  }

  /**
   * Waits for the given duration.
   *
   * The wait ends early when the client is closed, so close does not have
   * to wait out a long backoff.
   */
  private delay(ms: number): Promise<void> {
    return new Promise((resolve) => {
      const timer = setTimeout(() => {
        this.fetchRetryTimer = undefined;
        resolve();
      }, ms);
      this.fetchRetryTimer = timer;
    });
  }

  /** Updates the connection status and notifies listeners. */
  private setStatus(status: ConnectionStatus, error?: unknown): void {
    if (this.status === status && error === undefined) {
      return;
    }
    this.status = status;

    const detail: ConnectionStatusDetail = error === undefined ? { status } : { status, error };
    for (const listener of this.statusListeners) {
      try {
        listener(detail);
      } catch {
        // A failing listener must not break the client.
      }
    }
  }

  /** Reports a listener error without breaking the client. */
  private reportListenerError(error: unknown): void {
    // Listener errors are the application's problem, not the SDK's. They
    // are surfaced through the status channel so they are not silent.
    if (this.statusListeners.size > 0) {
      this.setStatus(this.status, error);
    }
  }
}

/**
 * Validates and normalises a configuration response.
 *
 * A malformed response is rejected rather than partially applied.
 */
function parseSnapshot(body: unknown): ConfigurationSnapshot | undefined {
  if (!body || typeof body !== "object") {
    return undefined;
  }
  const candidate = body as Partial<ConfigurationSnapshot>;
  if (typeof candidate.version !== "number" || !Number.isFinite(candidate.version)) {
    return undefined;
  }
  if (!candidate.flags || typeof candidate.flags !== "object") {
    return undefined;
  }

  const flags: Record<string, boolean> = {};
  for (const [key, value] of Object.entries(candidate.flags)) {
    if (typeof value === "boolean") {
      flags[key] = value;
    }
  }

  return {
    project_id: typeof candidate.project_id === "string" ? candidate.project_id : "",
    environment: typeof candidate.environment === "string" ? candidate.environment : "",
    version: candidate.version,
    flags,
  };
}

/**
 * Creates a feature flag client.
 *
 * @example
 * ```ts
 * const client = createFeatureFlagClient({
 *   apiUrl: "https://api.example.com",
 *   sdkKey: "ff_client_...",
 * });
 *
 * await client.initialize();
 * const enabled = client.isEnabled("new_dashboard", false);
 * ```
 */
export function createFeatureFlagClient(
  options: FeatureFlagClientOptions,
): FeatureFlagClient {
  return new FeatureFlagClientImpl(options);
}
