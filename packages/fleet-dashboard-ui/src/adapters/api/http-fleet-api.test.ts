import { describe, expect, it, vi } from "vitest";

import { createHttpFleetApi, HttpError } from "./http-fleet-api";

function okResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

describe("createHttpFleetApi", () => {
  it("requests the fleet route under the injected base path", async () => {
    // /api/fleet is a map keyed by instance name on the wire; the mapper flattens
    // it to a sorted list.
    const fetchImpl = vi.fn().mockResolvedValue(
      okResponse({
        data: {},
        generatedAt: "2026-01-01T00:00:00Z",
        stale: false,
        error: null,
      }),
    );
    const api = createHttpFleetApi({ basePath: "/api", fetchImpl });

    const plane = await api.getFleet();

    expect(fetchImpl).toHaveBeenCalledWith(
      "/api/fleet",
      expect.objectContaining({ headers: { Accept: "application/json" } }),
    );
    expect(plane.data.instances).toEqual([]);
  });

  it("strips a trailing slash from the base path", async () => {
    const fetchImpl = vi.fn().mockResolvedValue(
      okResponse({
        data: { order: [], environments: {}, releases: [] },
        generatedAt: "2026-01-01T00:00:00Z",
        stale: false,
        error: null,
      }),
    );
    const api = createHttpFleetApi({ basePath: "/api/", fetchImpl });

    await api.getPromotion();

    expect(fetchImpl).toHaveBeenCalledWith("/api/promotion", expect.anything());
  });

  it("forwards the abort signal", async () => {
    const controller = new AbortController();
    const fetchImpl = vi.fn().mockResolvedValue(
      okResponse({
        data: {},
        generatedAt: "2026-01-01T00:00:00Z",
        stale: false,
        error: null,
      }),
    );
    const api = createHttpFleetApi({ basePath: "/api", fetchImpl });

    await api.getTopology(controller.signal);

    expect(fetchImpl).toHaveBeenCalledWith(
      "/api/topology",
      expect.objectContaining({ signal: controller.signal }),
    );
  });

  // Verifies: SDLC-VIS-06.02
  it("requests the SDLC plane at /sdlc and maps the envelope", async () => {
    const fetchImpl = vi.fn().mockResolvedValue(
      okResponse({
        data: {
          phases: [{ key: "spec", sleSeconds: 10 }],
          cards: [],
          lane: [],
          matrix: { hubClusters: [], rows: [] },
        },
        generatedAt: "2026-01-01T00:00:00Z",
        stale: true,
        error: "upstream hiccup",
      }),
    );
    const api = createHttpFleetApi({ basePath: "/api", fetchImpl });

    const plane = await api.getSdlc();

    expect(fetchImpl).toHaveBeenCalledWith("/api/sdlc", expect.anything());
    // Envelope fields pass through unchanged; only data is projected to the domain.
    expect(plane.stale).toBe(true);
    expect(plane.error).toBe("upstream hiccup");
    expect(plane.data.phases[0]?.key).toBe("spec");
  });

  it("throws HttpError on a non-2xx response", async () => {
    const fetchImpl = vi
      .fn()
      .mockResolvedValue(new Response("nope", { status: 503 }));
    const api = createHttpFleetApi({ basePath: "/api", fetchImpl });

    await expect(api.getInstances()).rejects.toBeInstanceOf(HttpError);
  });

  it("requests recent e2e runs and maps the enabled flag", async () => {
    const fetchImpl = vi.fn().mockResolvedValue(
      okResponse({
        data: { enabled: true, runs: [] },
        generatedAt: "2026-01-01T00:00:00Z",
        stale: false,
        error: null,
      }),
    );
    const api = createHttpFleetApi({ basePath: "/api", fetchImpl });

    const plane = await api.getE2eRuns();

    expect(fetchImpl).toHaveBeenCalledWith("/api/e2e/runs", expect.anything());
    expect(plane.data.enabled).toBe(true);
  });

  it("url-encodes each segment of the multi-segment run id in the path", async () => {
    const fetchImpl = vi.fn().mockResolvedValue(
      okResponse({
        data: { id: "tenant-1/ns/run 1" },
        generatedAt: "2026-01-01T00:00:00Z",
        stale: false,
        error: null,
      }),
    );
    const api = createHttpFleetApi({ basePath: "/api", fetchImpl });

    await api.getE2eRun("tenant-1/ns/run 1");

    // The space in the run id is encoded, but the slash separators are preserved.
    expect(fetchImpl).toHaveBeenCalledWith(
      "/api/e2e/runs/tenant-1/ns/run%201",
      expect.anything(),
    );
  });
});

/** A controllable EventSource double for the SSE stream tests. */
class FakeEventSource {
  static instances: FakeEventSource[] = [];
  onmessage: ((ev: MessageEvent) => void) | null = null;
  onerror: ((ev: Event) => void) | null = null;
  closed = false;
  constructor(
    readonly url: string,
    readonly init?: EventSourceInit,
  ) {
    FakeEventSource.instances.push(this);
  }
  emit(data: unknown): void {
    this.onmessage?.({ data: JSON.stringify(data) } as MessageEvent);
  }
  fail(): void {
    this.onerror?.(new Event("error"));
  }
  close(): void {
    this.closed = true;
  }
}

describe("createHttpFleetApi streamE2eRun", () => {
  it("decodes event and status frames, and cancels via the unsubscribe thunk", () => {
    FakeEventSource.instances = [];
    const api = createHttpFleetApi({
      basePath: "/api",
      eventSourceImpl: FakeEventSource as unknown as typeof EventSource,
    });

    const events: string[] = [];
    const statuses: string[] = [];
    const unsubscribe = api.streamE2eRun("tenant-1/ns/run-1", {
      onEvent: (event) => {
        events.push(`${event.event}:${String(event.sequence)}`);
      },
      onStatus: (status) => {
        statuses.push(status.state);
      },
    });

    const es = FakeEventSource.instances[0];
    if (!es) throw new Error("expected an EventSource");
    expect(es.url).toBe("/api/e2e/stream?run=tenant-1%2Fns%2Frun-1");
    expect(es.init?.withCredentials).toBe(true);

    es.emit({ kind: "status", status: { state: "live" } });
    es.emit({
      kind: "event",
      event: { event: "step_started", sequence: 2, stepId: "a" },
    });
    // A malformed / unknown-name event is isolated, not delivered.
    es.emit({ kind: "event", event: { event: "mystery", sequence: 3 } });
    es.emit({ kind: "heartbeat" });

    expect(events).toEqual(["step_started:2"]);
    expect(statuses).toEqual(["live"]);

    unsubscribe();
    expect(es.closed).toBe(true);
  });

  it("reports disconnected on a transport error (never leaves it reading as live)", () => {
    FakeEventSource.instances = [];
    const api = createHttpFleetApi({
      basePath: "/api",
      eventSourceImpl: FakeEventSource as unknown as typeof EventSource,
    });
    const statuses: string[] = [];
    api.streamE2eRun("tenant-1/ns/run-1", {
      onEvent: () => {
        /* unused */
      },
      onStatus: (status) => {
        statuses.push(status.state);
      },
    });
    FakeEventSource.instances[0]?.fail();
    expect(statuses).toEqual(["disconnected"]);
  });

  // Verifies: E2E-LOG-11.01
  it("closes the stream when the provided abort signal fires", () => {
    FakeEventSource.instances = [];
    const api = createHttpFleetApi({
      basePath: "/api",
      eventSourceImpl: FakeEventSource as unknown as typeof EventSource,
    });
    const controller = new AbortController();
    api.streamE2eRun(
      "tenant-1/ns/run-1",
      {
        onEvent: () => {
          /* unused */
        },
        onStatus: () => {
          /* unused */
        },
      },
      controller.signal,
    );
    controller.abort();
    expect(FakeEventSource.instances[0]?.closed).toBe(true);
  });
});
