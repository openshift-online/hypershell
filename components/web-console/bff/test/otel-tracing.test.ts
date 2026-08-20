import { ExportResultCode } from "@opentelemetry/core";
import { OTLPTraceExporter } from "@opentelemetry/exporter-trace-otlp-http";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { TracingConfig } from "../src/config.js";
import {
  createBffTracing,
  routeTemplateFrom,
} from "../src/adapters/observability/otel-tracing.js";

const validSpan = {
  traceId: "0af7651916cd43dd8448eb211c80319c",
  spanId: "b7ad6b7169203331",
  name: "GET /api/hypershell/v1/gateways",
};

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
    expect(tracing.deliveryHealth()).toEqual({
      relayFailures: 0,
      spanExportFailures: 0,
    });
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

  it("continues a future-version traceparent, normalizing it to version 00", () => {
    const tracing = createBffTracing(config);

    // A version beyond 00 with trailing fields must still be honored per the W3C
    // spec; the propagated header is re-emitted at the version this BFF speaks.
    const futureVersion = `01-${inboundTraceId}-b7ad6b7169203331-01-extra`;
    const upstream = tracing
      .startProxySpan(proxyInput({ traceparent: futureVersion }))
      .upstream();

    expect(upstream?.traceparent).toMatch(
      new RegExp(`^00-${inboundTraceId}-[0-9a-f]{16}-01$`),
    );
  });

  it("starts a new trace for an all-zero or unsupported-version traceparent", () => {
    const tracing = createBffTracing(config);

    const allZero = "00-00000000000000000000000000000000-0000000000000000-01";
    const badVersion =
      "ff-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01";
    for (const traceparent of [allZero, badVersion]) {
      const upstream = tracing
        .startProxySpan(proxyInput({ traceparent }))
        .upstream();
      expect(upstream?.traceparent).toMatch(
        /^00-[0-9a-f]{32}-[0-9a-f]{16}-01$/,
      );
      expect(upstream?.traceparent).not.toContain(inboundTraceId);
    }
  });

  it("drops a malformed tracestate while still continuing the trace", () => {
    const tracing = createBffTracing(config);

    const upstream = tracing
      .startProxySpan(
        proxyInput({
          traceparent: validTraceparent,
          tracestate: "no-equals-sign",
        }),
      )
      .upstream();

    expect(upstream?.traceparent).toMatch(
      new RegExp(`^00-${inboundTraceId}-[0-9a-f]{16}-01$`),
    );
    expect(upstream?.tracestate).toBeUndefined();
  });

  it("rejects a payload that is not well-formed OTLP", async () => {
    const fetchSpy = vi.spyOn(globalThis, "fetch");
    const tracing = createBffTracing(config);

    await expect(tracing.ingestTraces({ notOtlp: true })).resolves.toBe(
      "rejected",
    );
    expect(fetchSpy).not.toHaveBeenCalled();
  });

  it("rejects a malformed OTLP envelope without relaying it", async () => {
    const fetchSpy = vi.spyOn(globalThis, "fetch");
    const tracing = createBffTracing(config);

    // resourceSpans present but not an array of objects.
    await expect(tracing.ingestTraces({ resourceSpans: [42] })).resolves.toBe(
      "rejected",
    );
    // scopeSpans is not an array.
    await expect(
      tracing.ingestTraces({ resourceSpans: [{ scopeSpans: "nope" }] }),
    ).resolves.toBe("rejected");
    // spans is not an array of objects.
    await expect(
      tracing.ingestTraces({
        resourceSpans: [{ scopeSpans: [{ spans: {} }] }],
      }),
    ).resolves.toBe("rejected");
    expect(fetchSpy).not.toHaveBeenCalled();
  });

  it("relays a fully nested OTLP envelope with spans", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(null, { status: 200 }),
    );
    const tracing = createBffTracing(config);

    await expect(
      tracing.ingestTraces({
        resourceSpans: [
          {
            scopeSpans: [
              {
                spans: [
                  {
                    traceId: "0af7651916cd43dd8448eb211c80319c",
                    spanId: "b7ad6b7169203331",
                    name: "s",
                  },
                ],
              },
            ],
          },
        ],
      }),
    ).resolves.toBe("accepted");
  });

  it("treats a collector 4xx as a rejection of the payload", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(null, { status: 400 }),
    );
    const tracing = createBffTracing(config);

    await expect(
      tracing.ingestTraces({ resourceSpans: [{ scopeSpans: [] }] }),
    ).resolves.toBe("rejected");
  });

  it("treats a transient collector 429 or 5xx as unavailable", async () => {
    const tracing = createBffTracing(config);

    const fetchSpy = vi.spyOn(globalThis, "fetch");
    for (const status of [429, 408, 503]) {
      fetchSpy.mockResolvedValueOnce(new Response(null, { status }));
      await expect(
        tracing.ingestTraces({ resourceSpans: [{ scopeSpans: [] }] }),
      ).resolves.toBe("unavailable");
    }
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

describe("BFF OTLP nested-field validation", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  function envelope(span: unknown): unknown {
    return { resourceSpans: [{ scopeSpans: [{ spans: [span] }] }] };
  }

  it("accepts a fully typed span and relays it", async () => {
    const fetchSpy = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(new Response(null, { status: 200 }));
    const tracing = createBffTracing(config);

    await expect(
      tracing.ingestTraces(
        envelope({
          ...validSpan,
          kind: 2,
          startTimeUnixNano: "1723000000000000000",
          endTimeUnixNano: 1_723_000_000_000_001,
          attributes: [{ key: "http.route", value: { stringValue: "/x" } }],
          status: { code: 1 },
        }),
      ),
    ).resolves.toBe("accepted");
    expect(fetchSpy).toHaveBeenCalledTimes(1);
  });

  it.each([
    ["a missing span id", { traceId: validSpan.traceId, name: "s" }],
    ["a non-hex span id", { ...validSpan, spanId: "not-hex-id-here!!" }],
    ["a non-hex trace id", { ...validSpan, traceId: "zz" }],
    ["a non-string name", { ...validSpan, name: 42 }],
    [
      "an attribute missing its key",
      { ...validSpan, attributes: [{ value: { stringValue: "x" } }] },
    ],
    ["a non-numeric span kind", { ...validSpan, kind: "server" }],
    ["an out-of-range span kind", { ...validSpan, kind: 99 }],
    ["an out-of-range status code", { ...validSpan, status: { code: 7 } }],
    [
      "a negative dropped-attributes count",
      { ...validSpan, droppedAttributesCount: -1 },
    ],
    ["a non-integer flags value", { ...validSpan, flags: 1.5 }],
    [
      "a malformed nanosecond timestamp",
      { ...validSpan, startTimeUnixNano: "12:00" },
    ],
    [
      "a uint64-overflow nanosecond timestamp",
      { ...validSpan, startTimeUnixNano: "18446744073709551616" },
    ],
    [
      "an unsafe-integer nanosecond timestamp number",
      { ...validSpan, startTimeUnixNano: 18_446_744_073_709_552_000 },
    ],
    [
      "a negative dropped-events count",
      { ...validSpan, droppedEventsCount: -1 },
    ],
    [
      "a non-integer dropped-events count",
      { ...validSpan, droppedEventsCount: 1.5 },
    ],
    ["a negative dropped-links count", { ...validSpan, droppedLinksCount: -1 }],
  ])("rejects a span with %s without relaying it", async (_label, span) => {
    const fetchSpy = vi.spyOn(globalThis, "fetch");
    const tracing = createBffTracing(config);

    await expect(tracing.ingestTraces(envelope(span))).resolves.toBe(
      "rejected",
    );
    expect(fetchSpy).not.toHaveBeenCalled();
  });

  it("accepts the exact 64-bit maxima and the dropped-collection counters", async () => {
    const fetchSpy = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(new Response(null, { status: 200 }));
    const tracing = createBffTracing(config);

    // The exact fixed-width boundaries are valid values the collector accepts;
    // only one-past-the-maximum is out of range. droppedEventsCount and
    // droppedLinksCount are real uint32 fields the JS exporter emits.
    await expect(
      tracing.ingestTraces(
        envelope({
          ...validSpan,
          startTimeUnixNano: "18446744073709551615",
          droppedEventsCount: 3,
          droppedLinksCount: 4,
          attributes: [
            { key: "int64max", value: { intValue: "9223372036854775807" } },
            { key: "int64min", value: { intValue: "-9223372036854775808" } },
          ],
        }),
      ),
    ).resolves.toBe("accepted");
    expect(fetchSpy).toHaveBeenCalledTimes(1);
  });

  it("accepts proto3 JSON scalar encodings in attribute values", async () => {
    const fetchSpy = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(new Response(null, { status: 200 }));
    const tracing = createBffTracing(config);

    await expect(
      tracing.ingestTraces(
        envelope({
          ...validSpan,
          attributes: [
            { key: "int", value: { intValue: "-42" } },
            { key: "bytes", value: { bytesValue: "AAAA" } },
            { key: "double", value: { doubleValue: "NaN" } },
            {
              key: "nested",
              value: { arrayValue: { values: [{ boolValue: true }] } },
            },
          ],
        }),
      ),
    ).resolves.toBe("accepted");
    expect(fetchSpy).toHaveBeenCalledTimes(1);
  });

  it.each([
    ["two set value fields (oneof)", { boolValue: true, stringValue: "x" }],
    ["a non-base64 bytes value", { bytesValue: "not base64!!" }],
    ["a non-integer int value", { intValue: "12.5" }],
    ["an int64-overflow int value", { intValue: "9223372036854775808" }],
    ["an int64-underflow int value", { intValue: "-9223372036854775809" }],
  ])(
    "rejects an attribute whose value has %s without relaying it",
    async (_label, value) => {
      const fetchSpy = vi.spyOn(globalThis, "fetch");
      const tracing = createBffTracing(config);

      await expect(
        tracing.ingestTraces(
          envelope({ ...validSpan, attributes: [{ key: "k", value }] }),
        ),
      ).resolves.toBe("rejected");
      expect(fetchSpy).not.toHaveBeenCalled();
    },
  );

  it("rejects a pathologically deep AnyValue without throwing", async () => {
    const fetchSpy = vi.spyOn(globalThis, "fetch");
    const tracing = createBffTracing(config);

    // Nest an AnyValue thousands of levels deep. A naive recursive validator
    // would overflow the call stack and throw; the bounded structural pre-pass
    // rejects it and the exception guard keeps any overflow from escaping.
    let value: unknown = { stringValue: "leaf" };
    for (let depth = 0; depth < 5_000; depth += 1) {
      value = { arrayValue: { values: [value] } };
    }

    await expect(
      tracing.ingestTraces(
        envelope({ ...validSpan, attributes: [{ key: "deep", value }] }),
      ),
    ).resolves.toBe("rejected");
    expect(fetchSpy).not.toHaveBeenCalled();
  });

  it.each([
    ["a body-sized run of significant digits", "9".repeat(1_048_000)],
    [
      "a body-sized run of leading zeros then out-of-range digits",
      `${"0".repeat(1_048_000)}${"1".repeat(21)}`,
    ],
  ])(
    "rejects a 64-bit string with %s quickly, without a body-sized BigInt parse",
    async (_label, timestamp) => {
      const fetchSpy = vi.spyOn(globalThis, "fetch");
      const tracing = createBffTracing(config);

      // A ~1 MiB decimal string fits under the body limit. Feeding it straight
      // to BigInt() is an O(n^2) parse that blocked the event loop for ~125 ms
      // even though the value is rejected; the significant-digit length guard
      // must reject it with only a linear scan. The generous ceiling separates
      // the linear path from the quadratic regression without flaking on slow
      // CI. The leading-zeros case (21 significant digits, out of range) proves
      // the guard strips zeros before the length bound and never builds a
      // body-sized BigInt.
      const startedAt = performance.now();
      await expect(
        tracing.ingestTraces(
          envelope({ ...validSpan, startTimeUnixNano: timestamp }),
        ),
      ).resolves.toBe("rejected");
      expect(performance.now() - startedAt).toBeLessThan(100);
      expect(fetchSpy).not.toHaveBeenCalled();
    },
  );
});

describe("BFF delivery health", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllEnvs();
  });

  function endOneSpan(tracing: ReturnType<typeof createBffTracing>): void {
    tracing.startProxySpan(proxyInput()).end("success", 200);
  }

  it("counts a failed span export and records its error type", async () => {
    vi.spyOn(OTLPTraceExporter.prototype, "export").mockImplementation(
      (_spans, resultCallback) => {
        resultCallback({
          code: ExportResultCode.FAILED,
          error: new Error("collector unreachable"),
        });
      },
    );
    vi.spyOn(OTLPTraceExporter.prototype, "shutdown").mockResolvedValue();
    const tracing = createBffTracing(config);

    endOneSpan(tracing);
    // Shutdown flushes the buffered span; the export fails and the processor's
    // self-observation meter folds the loss into the health tally. The flush
    // rejects on the failed export, but accounting happens before it rejects.
    await tracing.shutdown().catch(() => undefined);

    const health = tracing.deliveryHealth();
    expect(health.spanExportFailures).toBeGreaterThanOrEqual(1);
    expect(health.lastErrorType).toBe("Error");
    expect(health.relayFailures).toBe(0);
  });

  it("counts a synchronous exporter throw as a delivery failure", async () => {
    // The exporter throws instead of calling back. Without the backstop's throw
    // guard the loss would go unaccounted and shutdown would report zero.
    const thrown = new Error("exporter blew up");
    thrown.name = "SyncExportError";
    vi.spyOn(OTLPTraceExporter.prototype, "export").mockImplementation(() => {
      throw thrown;
    });
    vi.spyOn(OTLPTraceExporter.prototype, "shutdown").mockResolvedValue();
    const tracing = createBffTracing(config);

    endOneSpan(tracing);
    await tracing.shutdown().catch(() => undefined);

    const health = tracing.deliveryHealth();
    expect(health.spanExportFailures).toBeGreaterThanOrEqual(1);
    expect(health.lastErrorType).toBe("SyncExportError");
  });

  it("counts every span in a failed multi-span batch, not just one", async () => {
    vi.spyOn(OTLPTraceExporter.prototype, "export").mockImplementation(
      (_spans, resultCallback) => {
        resultCallback({
          code: ExportResultCode.FAILED,
          error: new Error("collector unreachable"),
        });
      },
    );
    vi.spyOn(OTLPTraceExporter.prototype, "shutdown").mockResolvedValue();
    const tracing = createBffTracing(config);

    // Three spans buffer and flush together as one failed batch; the tally must
    // advance by the batch's span count rather than by a single increment.
    for (let index = 0; index < 3; index += 1) {
      endOneSpan(tracing);
    }
    await tracing.shutdown().catch(() => undefined);

    expect(tracing.deliveryHealth().spanExportFailures).toBe(3);
  });

  it("counts spans dropped by an overflowing queue", () => {
    // The shim reads OTEL_BSP_* env for its bounds; a one-slot queue plus an
    // exporter that never drains forces every further span to overflow.
    vi.stubEnv("OTEL_BSP_MAX_QUEUE_SIZE", "1");
    vi.stubEnv("OTEL_BSP_MAX_EXPORT_BATCH_SIZE", "1");
    vi.stubEnv("OTEL_BSP_SCHEDULE_DELAY", "1");
    vi.spyOn(OTLPTraceExporter.prototype, "export").mockImplementation(
      () => undefined,
    );
    const tracing = createBffTracing(config);

    for (let index = 0; index < 8; index += 1) {
      endOneSpan(tracing);
    }

    const health = tracing.deliveryHealth();
    expect(health.spanExportFailures).toBeGreaterThanOrEqual(1);
    expect(health.lastErrorType).toBe("queue_full");
  });

  it("counts a wedged exporter that never acknowledges a batch", async () => {
    // The exporter accepts the batch and never calls back. The backstop turns
    // the stall into a FAILED result the self-observation meter records.
    vi.useFakeTimers();
    try {
      vi.spyOn(OTLPTraceExporter.prototype, "export").mockImplementation(
        () => undefined,
      );
      vi.spyOn(OTLPTraceExporter.prototype, "shutdown").mockResolvedValue();
      const tracing = createBffTracing(config);

      endOneSpan(tracing);
      const shutdown = tracing.shutdown().catch(() => undefined);
      await vi.advanceTimersByTimeAsync(15_000);
      await shutdown;

      const health = tracing.deliveryHealth();
      expect(health.spanExportFailures).toBeGreaterThanOrEqual(1);
      expect(health.lastErrorType).toBe("SpanExportTimeout");
    } finally {
      vi.useRealTimers();
    }
  });

  it("counts a browser relay that cannot reach the collector", async () => {
    vi.spyOn(globalThis, "fetch").mockRejectedValue(new Error("unreachable"));
    const tracing = createBffTracing(config);

    await tracing.ingestTraces({ resourceSpans: [{ scopeSpans: [] }] });

    const health = tracing.deliveryHealth();
    expect(health.relayFailures).toBe(1);
    expect(health.lastErrorType).toBe("collector_unreachable");
    expect(health.spanExportFailures).toBe(0);
  });

  it("counts a transient collector response as a relay failure", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(null, { status: 503 }),
    );
    const tracing = createBffTracing(config);

    await tracing.ingestTraces({ resourceSpans: [{ scopeSpans: [] }] });

    const health = tracing.deliveryHealth();
    expect(health.relayFailures).toBe(1);
    expect(health.lastErrorType).toBe("collector_unavailable");
  });

  it("does not count a collector 4xx rejection as a relay failure", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(null, { status: 400 }),
    );
    const tracing = createBffTracing(config);

    await tracing.ingestTraces({ resourceSpans: [{ scopeSpans: [] }] });

    expect(tracing.deliveryHealth()).toEqual({
      relayFailures: 0,
      spanExportFailures: 0,
    });
  });

  it("reports zero failures after a healthy export and shutdown", async () => {
    vi.spyOn(OTLPTraceExporter.prototype, "export").mockImplementation(
      (_spans, resultCallback) => {
        resultCallback({ code: ExportResultCode.SUCCESS });
      },
    );
    vi.spyOn(OTLPTraceExporter.prototype, "shutdown").mockResolvedValue();
    const tracing = createBffTracing(config);

    endOneSpan(tracing);
    await tracing.shutdown();

    expect(tracing.deliveryHealth()).toEqual({
      relayFailures: 0,
      spanExportFailures: 0,
    });
  });
});
