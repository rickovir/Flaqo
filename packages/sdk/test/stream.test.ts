import { describe, expect, it, vi } from "vitest";
import { createFeatureFlagClient } from "../src/index.js";
import type { ConfigurationSnapshot } from "../src/index.js";

/** Builds an SSE frame. */
function sseFrame(event: string, data: string, id?: string): string {
  const lines: string[] = [];
  if (id !== undefined) {
    lines.push(`id: ${id}`);
  }
  lines.push(`event: ${event}`);
  lines.push(`data: ${data}`);
  return `${lines.join("\n")}\n\n`;
}

/**
 * A stream whose frames are pushed by the test.
 *
 * The controller is exposed so a test can deliver events at the moment
 * it chooses.
 */
function createControllableStream() {
  let controller: ReadableStreamDefaultController<Uint8Array> | undefined;

  const stream = new ReadableStream<Uint8Array>({
    start(c) {
      controller = c;
    },
  });

  const encoder = new TextEncoder();

  return {
    stream,
    push(text: string) {
      controller?.enqueue(encoder.encode(text));
    },
    close() {
      controller?.close();
    },
  };
}

interface StubOptions {
  /** Snapshots returned by the config endpoint, in order. */
  configs?: Array<ConfigurationSnapshot | { status: number }>;
  /** The stream body. */
  stream?: ReadableStream<Uint8Array>;
}

/**
 * A transport stub with a config queue and a controllable stream.
 *
 * The config endpoint pops from the queue, so a test can make the first
 * fetch fail and the second succeed.
 */
function createStub(options: StubOptions = {}) {
  const configs = [...(options.configs ?? [])];
  let configCalls = 0;
  let streamCalls = 0;

  const stub = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = typeof input === "string" ? input : input.toString();

    if (url.includes("/sdk/v1/config")) {
      configCalls += 1;
      const next = configs.length > 0 ? configs.shift() : undefined;
      if (next === undefined) {
        return new Response(JSON.stringify({
          project_id: "p1",
          environment: "production",
          version: 1,
          flags: { a: true },
        }), { status: 200 });
      }
      if ("status" in next) {
        return new Response(null, { status: next.status });
      }
      return new Response(JSON.stringify(next), { status: 200 });
    }

    if (url.includes("/sdk/v1/stream")) {
      streamCalls += 1;
      const body =
        options.stream ??
        new ReadableStream<Uint8Array>({
          start() {
            // Keep the stream open until the client aborts it.
          },
        });
      void init;
      return new Response(body, {
        status: 200,
        headers: { "Content-Type": "text/event-stream" },
      });
    }

    throw new Error(`unexpected request to ${url}`);
  });

  return {
    fetch: stub as unknown as typeof globalThis.fetch,
    mock: stub,
    configCalls: () => configCalls,
    streamCalls: () => streamCalls,
  };
}

/** Waits for a condition, polling until it holds or the timeout expires. */
async function waitFor(
  condition: () => boolean,
  timeoutMs = 3_000,
): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (condition()) {
      return;
    }
    await new Promise((resolve) => setTimeout(resolve, 10));
  }
  throw new Error("condition was not met within the timeout");
}

describe("update stream", () => {
  it("opens the stream after the initial snapshot is applied", async () => {
    const stub = createStub();
    const client = createFeatureFlagClient({
      apiUrl: "https://api.test",
      sdkKey: "ff_client_test",
      fetch: stub.fetch,
      reconnectBaseDelayMs: 10,
      reconnectMaxDelayMs: 50,
    });

    await client.initialize();

    await waitFor(() => stub.streamCalls() >= 1);
    expect(stub.streamCalls()).toBeGreaterThanOrEqual(1);
    await client.close();
  });

  it("uses fetch with a stream reader rather than EventSource", async () => {
    const stub = createStub();
    const client = createFeatureFlagClient({
      apiUrl: "https://api.test",
      sdkKey: "ff_client_test",
      fetch: stub.fetch,
      reconnectBaseDelayMs: 10,
      reconnectMaxDelayMs: 50,
    });

    await client.initialize();
    await waitFor(() => stub.streamCalls() >= 1);

    // The stream request carries the credential in the Authorization
    // header. EventSource could not do this.
    const streamCall = stub.mock.mock.calls.find((call) =>
      String(call[0]).includes("/sdk/v1/stream"),
    );
    const init = streamCall?.[1] as RequestInit | undefined;
    const headers = init?.headers as Record<string, string> | undefined;
    expect(headers?.Authorization).toBe("FF ff_client_test");

    await client.close();
  });

  it("fetches the snapshot when a newer version arrives", async () => {
    const controllable = createControllableStream();
    const stub = createStub({
      configs: [
        { project_id: "p1", environment: "production", version: 1, flags: { a: true } },
        { project_id: "p1", environment: "production", version: 2, flags: { a: false } },
      ],
      stream: controllable.stream,
    });

    const client = createFeatureFlagClient({
      apiUrl: "https://api.test",
      sdkKey: "ff_client_test",
      fetch: stub.fetch,
      reconnectBaseDelayMs: 10,
      reconnectMaxDelayMs: 50,
    });

    await client.initialize();
    expect(client.getVersion()).toBe(1);
    expect(client.isEnabled("a")).toBe(true);

    await waitFor(() => stub.streamCalls() >= 1);
    controllable.push(sseFrame("config.updated", '{"version":2}', "2"));

    await waitFor(() => client.getVersion() === 2);
    expect(client.isEnabled("a")).toBe(false);

    await client.close();
  });

  it("ignores an event with an older version", async () => {
    const controllable = createControllableStream();
    const stub = createStub({
      configs: [
        { project_id: "p1", environment: "production", version: 5, flags: { a: true } },
      ],
      stream: controllable.stream,
    });

    const client = createFeatureFlagClient({
      apiUrl: "https://api.test",
      sdkKey: "ff_client_test",
      fetch: stub.fetch,
      reconnectBaseDelayMs: 10,
      reconnectMaxDelayMs: 50,
    });

    await client.initialize();
    expect(client.getVersion()).toBe(5);

    await waitFor(() => stub.streamCalls() >= 1);
    const callsBefore = stub.configCalls();

    // An older version must be ignored.
    controllable.push(sseFrame("config.updated", '{"version":2}', "2"));
    controllable.push(sseFrame("config.updated", '{"version":5}', "5"));

    await new Promise((resolve) => setTimeout(resolve, 100));
    expect(stub.configCalls()).toBe(callsBefore);
    expect(client.getVersion()).toBe(5);

    await client.close();
  });

  it("ignores a heartbeat comment", async () => {
    const controllable = createControllableStream();
    const stub = createStub({ stream: controllable.stream });

    const client = createFeatureFlagClient({
      apiUrl: "https://api.test",
      sdkKey: "ff_client_test",
      fetch: stub.fetch,
      reconnectBaseDelayMs: 10,
      reconnectMaxDelayMs: 50,
    });

    await client.initialize();
    await waitFor(() => stub.streamCalls() >= 1);

    const callsBefore = stub.configCalls();
    controllable.push(": heartbeat\n\n");

    await new Promise((resolve) => setTimeout(resolve, 100));
    expect(stub.configCalls()).toBe(callsBefore);

    await client.close();
  });

  it("retries a failed snapshot fetch even when no further event arrives", async () => {
    const controllable = createControllableStream();
    const stub = createStub({
      configs: [
        { project_id: "p1", environment: "production", version: 1, flags: { a: true } },
        // The fetch triggered by the event fails.
        { status: 500 },
        // The retry succeeds.
        { project_id: "p1", environment: "production", version: 2, flags: { a: false } },
      ],
      stream: controllable.stream,
    });

    const client = createFeatureFlagClient({
      apiUrl: "https://api.test",
      sdkKey: "ff_client_test",
      fetch: stub.fetch,
      reconnectBaseDelayMs: 10,
      reconnectMaxDelayMs: 50,
    });

    await client.initialize();
    expect(client.getVersion()).toBe(1);

    await waitFor(() => stub.streamCalls() >= 1);
    controllable.push(sseFrame("config.updated", '{"version":2}', "2"));

    // The retry must eventually succeed, without any further event. If
    // the retry depended on the next event, the client would stay on
    // version 1 forever.
    await waitFor(() => client.getVersion() === 2, 5_000);
    expect(client.isEnabled("a")).toBe(false);

    await client.close();
  });

  it("retains the last valid snapshot when the stream ends", async () => {
    const controllable = createControllableStream();
    const stub = createStub({ stream: controllable.stream });

    const client = createFeatureFlagClient({
      apiUrl: "https://api.test",
      sdkKey: "ff_client_test",
      fetch: stub.fetch,
      reconnectBaseDelayMs: 10,
      reconnectMaxDelayMs: 50,
    });

    await client.initialize();
    expect(client.isEnabled("a")).toBe(true);

    await waitFor(() => stub.streamCalls() >= 1);
    controllable.close();

    // The snapshot survives the stream ending.
    await new Promise((resolve) => setTimeout(resolve, 100));
    expect(client.isEnabled("a")).toBe(true);

    await client.close();
  });

  it("reconnects after the stream ends", async () => {
    const controllable = createControllableStream();
    const stub = createStub({ stream: controllable.stream });

    const client = createFeatureFlagClient({
      apiUrl: "https://api.test",
      sdkKey: "ff_client_test",
      fetch: stub.fetch,
      reconnectBaseDelayMs: 10,
      reconnectMaxDelayMs: 50,
    });

    await client.initialize();
    await waitFor(() => stub.streamCalls() >= 1);

    controllable.close();

    // The client must reconnect rather than give up.
    await waitFor(() => stub.streamCalls() >= 2, 3_000);
    await client.close();
  });

  it("stops reconnecting after close", async () => {
    const controllable = createControllableStream();
    const stub = createStub({ stream: controllable.stream });

    const client = createFeatureFlagClient({
      apiUrl: "https://api.test",
      sdkKey: "ff_client_test",
      fetch: stub.fetch,
      reconnectBaseDelayMs: 10,
      reconnectMaxDelayMs: 50,
    });

    await client.initialize();
    await waitFor(() => stub.streamCalls() >= 1);

    await client.close();
    const callsAfterClose = stub.streamCalls();

    await new Promise((resolve) => setTimeout(resolve, 200));
    expect(stub.streamCalls()).toBe(callsAfterClose);
  });
});
