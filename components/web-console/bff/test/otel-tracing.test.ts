import { afterEach, describe, expect, it, vi } from "vitest";

import type { TracingConfig } from "../src/config.js";
import {
  createBffTracing,
  routeTemplateFrom,
} from "../src/adapters/observability/otel-tracing.js";

const config: TracingConfig = {
  collectorEndpoint: "http://collector.test:4318",
  sampleRatio: 1,
  serviceName: "web-console-bff",
  tracesEndpoint: "http://collector.test:4318/v1/traces",
};

const validTraceparent =
  "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01";
const inboundTraceId = "0af7651916cd43dd8448eb211c80319c";

function proxyInput(
  overrides: Partial<
    Parameters<ReturnType<typeof createBffTracing>["startProxySpan"]>[0]
  > = {},
) {
  return {
    correlationId: "11111111-1111-4111-8111-111111111111",
    method: "GET",
    path: "/api/hypershell/v1/gateways",
    ...overrides,
  };
}

describe("routeTemplateFrom", () => {
  it("keeps a collection path literal", () => {
    expect(routeTemplateFrom("/api/hypershell/v1/gateways")).toBe(
      "/api/hypershell/v1/gateways",
    );
  });

  it("collapses a resource id after the collection to {id}", () => {
    expect(
      routeTemplateFrom("/api/hypershell/v1/gateways/01HZY8_ABC123DEF456GH"),
    ).toBe("/api/hypershell/v1/gateways/{id}");
  });

  it("collapses a numeric id and keeps a trailing action literal", () => {
    expect(routeTemplateFrom("/api/hypershell/v1/fleets/42/rename")).toBe(
      "/api/hypershell/v1/fleets/{id}/rename",
    );
  });

  it("never collapses the versioned api prefix", () => {
    expect(routeTemplateFrom("/api/hypershell/v1/metadata")).toBe(
      "/api/hypershell/v1/metadata",
    );
  });

  it("collapses id-shaped segments when no version is present", () => {
    expect(routeTemplateFrom("/gateways/01HZY8ABC123DEF456GHJKMN")).toBe(
      "/gateways/{id}",
    );
  });
});

describe("BFF tracing adapter", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("returns a disabled port when no collector is configured", async () => {
    const tracing = createBffTracing(undefined);

    expect(tracing.enabled).toBe(false);
    expect(tracing.startProxySpan(proxyInput()).upstream()).toBeUndefined();
    await expect(tracing.ingestTraces({ resourceSpans: [] })).resolves.toBe(
      "unavailable",
    );
  });

  it("continues a valid inbound trace and references its own span upstream", () => {
    const tracing = createBffTracing(config);

    const span = tracing.startProxySpan(
      proxyInput({ traceparent: validTraceparent }),
    );
    const upstream = span.upstream();

    expect(upstream?.traceparent).toMatch(
      new RegExp(`^00-${inboundTraceId}-[0-9a-f]{16}-01$`),
    );
    // The upstream span id references the BFF span, not the inbound one.
    expect(upstream?.traceparent).not.toContain("b7ad6b7169203331");
  });

  it("starts a new trace when the inbound traceparent is malformed", () => {
    const tracing = createBffTracing(config);

    const upstream = tracing
      .startProxySpan(proxyInput({ traceparent: "not-a-traceparent" }))
      .upstream();

    expect(upstream?.traceparent).toMatch(/^00-[0-9a-f]{32}-[0-9a-f]{16}-01$/);
    expect(upstream?.traceparent).not.toContain(inboundTraceId);
    expect(upstream?.tracestate).toBeUndefined();
  });

  it("forwards a valid tracestate only alongside a valid parent", () => {
    const tracing = createBffTracing(config);

    const continued = tracing
      .startProxySpan(
        proxyInput({ traceparent: validTraceparent, tracestate: "vendor=1" }),
      )
      .upstream();
    const orphaned = tracing
      .startProxySpan(
        proxyInput({ traceparent: "bad", tracestate: "vendor=1" }),
      )
      .upstream();

    expect(continued?.tracestate).toBe("vendor=1");
    expect(orphaned?.tracestate).toBeUndefined();
  });

  it("rejects a payload that is not well-formed OTLP", async () => {
    const fetchSpy = vi.spyOn(globalThis, "fetch");
    const tracing = createBffTracing(config);

    await expect(tracing.ingestTraces({ notOtlp: true })).resolves.toBe(
      "rejected",
    );
    expect(fetchSpy).not.toHaveBeenCalled();
  });

  it("relays a well-formed OTLP payload to the collector", async () => {
    const fetchSpy = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(new Response(null, { status: 200 }));
    const tracing = createBffTracing(config);

    await expect(
      tracing.ingestTraces({ resourceSpans: [{ scopeSpans: [] }] }),
    ).resolves.toBe("accepted");

    const [url, init] = fetchSpy.mock.calls[0] ?? [];
    expect(url).toBe(config.tracesEndpoint);
    expect(init?.method).toBe("POST");
    expect(new Headers(init?.headers).get("content-type")).toBe(
      "application/json",
    );
    const body = init?.body;
    expect(typeof body).toBe("string");
    expect(body as string).toContain("resourceSpans");
  });

  it("reports the collector as unavailable rather than failing", async () => {
    vi.spyOn(globalThis, "fetch").mockRejectedValue(new Error("unreachable"));
    const tracing = createBffTracing(config);

    await expect(tracing.ingestTraces({ resourceSpans: [] })).resolves.toBe(
      "unavailable",
    );
  });
});
