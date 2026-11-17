/**
 * Public types for the feature flag SDK.
 */

/**
 * A complete configuration snapshot for one project and environment.
 *
 * This mirrors the API's configuration contract. It contains only flag
 * keys, Boolean values, and the metadata the SDK needs.
 */
export interface ConfigurationSnapshot {
  project_id: string;
  environment: string;
  version: number;
  flags: Record<string, boolean>;
}

/** Options for creating a client. */
export interface FeatureFlagClientOptions {
  /** Base URL of the API, for example https://api.example.com. */
  apiUrl: string;
  /**
   * The scoped SDK key.
   *
   * Client keys are visible to anyone using the application and grant
   * read-only access to one project and environment. Server keys are
   * confidential and must never be included in a browser bundle.
   */
  sdkKey: string;
  /**
   * Initial backoff for reconnect attempts, in milliseconds.
   *
   * @defaultValue 1000
   */
  reconnectBaseDelayMs?: number;
  /**
   * Maximum backoff for reconnect attempts, in milliseconds.
   *
   * @defaultValue 30000
   */
  reconnectMaxDelayMs?: number;
  /**
   * Timeout for configuration fetches, in milliseconds.
   *
   * @defaultValue 10000
   */
  requestTimeoutMs?: number;
  /**
   * Override the fetch implementation. Useful in tests and in runtimes
   * with a customised HTTP stack.
   */
  fetch?: typeof globalThis.fetch;
}

/** The connection status of the update stream. */
export type ConnectionStatus =
  | "idle"
  | "connecting"
  | "connected"
  | "reconnecting"
  | "closed";

/** Detail about a connection status change. */
export interface ConnectionStatusDetail {
  status: ConnectionStatus;
  /** The error that caused the transition, when there was one. */
  error?: unknown;
}

/** A listener notified when the configuration snapshot changes. */
export type ChangeListener = (snapshot: ConfigurationSnapshot) => void;

/** A listener notified when the connection status changes. */
export type StatusListener = (detail: ConnectionStatusDetail) => void;

/** Removes a registered listener. */
export type Unsubscribe = () => void;

/** The public interface of a feature flag client. */
export interface FeatureFlagClient {
  /**
   * Fetches the initial configuration before resolving.
   *
   * The update stream starts only after the first snapshot is applied.
   */
  initialize(): Promise<void>;

  /**
   * Reads a flag from the local snapshot.
   *
   * This never performs a network request. It returns the fallback when
   * the flag is absent, or when the client has not initialized.
   */
  isEnabled(key: string, fallback?: boolean): boolean;

  /**
   * Registers a listener notified after a new snapshot is applied.
   */
  on(event: "change", listener: ChangeListener): Unsubscribe;
  on(event: "status", listener: StatusListener): Unsubscribe;

  /** The current connection status. */
  getStatus(): ConnectionStatus;

  /** The currently applied snapshot, or undefined before initialization. */
  getSnapshot(): ConfigurationSnapshot | undefined;

  /** The configuration version currently applied. */
  getVersion(): number | undefined;

  /**
   * Stops the update stream and releases resources.
   *
   * After close, the last snapshot remains readable, so local evaluation
   * keeps working.
   */
  close(): Promise<void>;
}
