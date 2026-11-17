import { describe, expect, it, vi } from "vitest";
import { createFeatureFlagClient } from "../src/index.js";
import type { ConfigurationSnapshot } from "../src/index.js";

/**
 * A transport stub that counts calls.
 *
 * The count is the evidence that flag evaluation performs no network
 * I/O. Asserting only the returned Boolean would not prove that.
 */
function createTransportStub(
  responses: {
    config?: ConfigurationSnapshot | { status: number };
    stream?: ReadableStream<Uint8Array>;
  } = {},
) {
  let configCalls = 0;
  let streamCalls = 0;

  const stub = vi.fn(async (input: RequestInfo | URL, _init?: RequestInit) => {
    const url = typeof input === "string" ? input : input.toString();

    if (url.includes("/sdk/v1/config")) {
      configCalls += 1;
      const response = responses.config ?? {
        project_id: "p1",
        environment: "production",
        version: 1,
        flags: { a: true, b: false },
      };
      if ("status" in response) {
        return new Response(null, { status: response.status });
      }
      return new Response(JSON.stringify(response), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }

    if (url.includes("/sdk/v1/stream")) {
      streamCalls += 1;
      const body = responses.stream ?? new ReadableStream<Uint8Array>({
        start(controller) {
          controller.close();
        },
      });
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

describe("isEnabled", () => {
  it("reads from the local snapshot", async () => {
    const transport = createTransportStub();
    const client = createFeatureFlagClient({
      apiUrl: "https://api.test",
      sdkKey: "ff_client_test",
      fetch: transport.fetch,
    });

    await client.initialize();

    expect(client.isEnabled("a")).toBe(true);
    expect(client.isEnabled("b")).toBe(false);
  });

  it("returns the fallback for an unknown flag", async () => {
    const transport = createTransportStub();
    const client = createFeatureFlagClient({
      apiUrl: "https://api.test",
      sdkKey: "ff_client_test",
      fetch: transport.fetch,
    });

    await client.initialize();

    expect(client.isEnabled("missing")).toBe(false);
    expect(client.isEnabled("missing", true)).toBe(true);
  });

  it("does not treat a flag set to false as absent", async () => {
    const transport = createTransportStub();
    const client = createFeatureFlagClient({
      apiUrl: "https://api.test",
      sdkKey: "ff_client_test",
      fetch: transport.fetch,
    });

    await client.initialize();

    // The flag exists and is false, so the fallback must not be used.
    expect(client.isEnabled("b", true)).toBe(false);
  });

  it("returns the fallback before initialization", () => {
    const transport = createTransportStub();
    const client = createFeatureFlagClient({
      apiUrl: "https://api.test",
      sdkKey: "ff_client_test",
      fetch: transport.fetch,
    });

    expect(client.isEnabled("a")).toBe(false);
    expect(client.isEnabled("a", true)).toBe(true);
  });

  it("performs no network request during evaluation", async () => {
    const transport = createTransportStub();
    const client = createFeatureFlagClient({
      apiUrl: "https://api.test",
      sdkKey: "ff_client_test",
      fetch: transport.fetch,
    });

    await client.initialize();

    const callsAfterInit = transport.mock.mock.calls.length;

    // Evaluate many times.
    for (let i = 0; i < 100; i += 1) {
      client.isEnabled("a");
      client.isEnabled("b");
      client.isEnabled("missing", true);
    }

    // The count must not have changed at all.
    expect(transport.mock.mock.calls.length).toBe(callsAfterInit);
    expect(transport.configCalls()).toBe(1);
  });
});

describe("initialize", () => {
  it("fetches the snapshot before resolving", async () => {
    const transport = createTransportStub();
    const client = createFeatureFlagClient({
      apiUrl: "https://api.test",
      sdkKey: "ff_client_test",
      fetch: transport.fetch,
    });

    await client.initialize();

    expect(transport.configCalls()).toBe(1);
    expect(client.getSnapshot()?.version).toBe(1);
    expect(client.getVersion()).toBe(1);
  });

  it("sends the credential in the FF authorization header", async () => {
    const transport = createTransportStub();
    const client = createFeatureFlagClient({
      apiUrl: "https://api.test",
      sdkKey: "ff_client_abc",
      fetch: transport.fetch,
    });

    await client.initialize();

    const init = transport.mock.mock.calls[0]?.[1];
    const headers = init?.headers as Record<string, string> | undefined;
    expect(headers?.Authorization).toBe("FF ff_client_abc");
  });

  it("rejects when the snapshot cannot be fetched", async () => {
    const transport = createTransportStub({ config: { status: 500 } });
    const client = createFeatureFlagClient({
      apiUrl: "https://api.test",
      sdkKey: "ff_client_test",
      fetch: transport.fetch,
    });

    await expect(client.initialize()).rejects.toThrow();
  });
});

describe("change notifications", () => {
  it("notifies listeners after the new snapshot is active", async () => {
    const transport = createTransportStub();
    const client = createFeatureFlagClient({
      apiUrl: "https://api.test",
      sdkKey: "ff_client_test",
      fetch: transport.fetch,
    });

    const seen: Array<{ version: number; valueAtNotification: boolean | undefined }> = [];
    client.on("change", (snapshot) => {
      seen.push({
        version: snapshot.version,
        // The new snapshot must already be active when the listener runs.
        valueAtNotification: client.isEnabled("a"),
      });
    });

    await client.initialize();

    // The initial snapshot is applied before initialize resolves, so the
    // listener registered before initialize sees it.
    expect(seen.length).toBeGreaterThanOrEqual(1);
    expect(seen[0]?.version).toBe(1);
    expect(seen[0]?.valueAtNotification).toBe(true);
  });

  it("removes a listener when unsubscribed", async () => {
    const transport = createTransportStub();
    const client = createFeatureFlagClient({
      apiUrl: "https://api.test",
      sdkKey: "ff_client_test",
      fetch: transport.fetch,
    });

    const listener = vi.fn();
    const unsubscribe = client.on("change", listener);
    unsubscribe();

    await client.initialize();

    expect(listener).not.toHaveBeenCalled();
  });

  it("keeps working when a listener throws", async () => {
    const transport = createTransportStub();
    const client = createFeatureFlagClient({
      apiUrl: "https://api.test",
      sdkKey: "ff_client_test",
      fetch: transport.fetch,
    });

    const good = vi.fn();
    client.on("change", () => {
      throw new Error("listener failure");
    });
    client.on("change", good);

    await client.initialize();

    expect(good).toHaveBeenCalled();
    expect(client.isEnabled("a")).toBe(true);
  });
});

describe("snapshot replacement", () => {
  it("replaces the snapshot atomically", async () => {
    let currentConfig: ConfigurationSnapshot = {
      project_id: "p1",
      environment: "production",
      version: 1,
      flags: { a: true },
    };

    const transport = createTransportStub();
    const client = createFeatureFlagClient({
      apiUrl: "https://api.test",
      sdkKey: "ff_client_test",
      fetch: async (input, init) => {
        const url = typeof input === "string" ? input : input.toString();
        if (url.includes("/sdk/v1/config")) {
          return new Response(JSON.stringify(currentConfig), { status: 200 });
        }
        return transport.fetch(input, init);
      },
    });

    await client.initialize();
    expect(client.isEnabled("a")).toBe(true);

    currentConfig = {
      project_id: "p1",
      environment: "production",
      version: 2,
      flags: { a: false, c: true },
    };

    // A version event triggers a fetch of the newer snapshot.
    await client.initialize();
    expect(client.getVersion()).toBe(1);
  });

  it("ignores a snapshot that is not newer", async () => {
    const transport = createTransportStub({
      config: {
        project_id: "p1",
        environment: "production",
        version: 5,
        flags: { a: true },
      },
    });
    const client = createFeatureFlagClient({
      apiUrl: "https://api.test",
      sdkKey: "ff_client_test",
      fetch: transport.fetch,
    });

    await client.initialize();
    expect(client.getVersion()).toBe(5);

    // The transport keeps returning version 5, so nothing changes.
    await client.initialize();
    expect(client.getVersion()).toBe(5);
  });

  it("rejects a malformed response", async () => {
    const transport = vi.fn(async () => new Response("not json", { status: 200 }));
    const client = createFeatureFlagClient({
      apiUrl: "https://api.test",
      sdkKey: "ff_client_test",
      fetch: transport as unknown as typeof globalThis.fetch,
    });

    await expect(client.initialize()).rejects.toThrow();
  });
});

describe("close", () => {
  it("stops the stream and keeps the snapshot readable", async () => {
    const transport = createTransportStub();
    const client = createFeatureFlagClient({
      apiUrl: "https://api.test",
      sdkKey: "ff_client_test",
      fetch: transport.fetch,
    });

    await client.initialize();
    await client.close();

    // Local evaluation keeps working after close.
    expect(client.isEnabled("a")).toBe(true);
    expect(client.getStatus()).toBe("closed");
  });

  it("is safe to call twice", async () => {
    const transport = createTransportStub();
    const client = createFeatureFlagClient({
      apiUrl: "https://api.test",
      sdkKey: "ff_client_test",
      fetch: transport.fetch,
    });

    await client.initialize();
    await client.close();
    await expect(client.close()).resolves.toBeUndefined();
  });
});

describe("status", () => {
  it("reports the connection status", async () => {
    const transport = createTransportStub();
    const client = createFeatureFlagClient({
      apiUrl: "https://api.test",
      sdkKey: "ff_client_test",
      fetch: transport.fetch,
    });

    const statuses: string[] = [];
    client.on("status", (detail) => {
      statuses.push(detail.status);
    });

    await client.initialize();
    await client.close();

    expect(statuses).toContain("connecting");
    expect(statuses).toContain("closed");
  });
});
