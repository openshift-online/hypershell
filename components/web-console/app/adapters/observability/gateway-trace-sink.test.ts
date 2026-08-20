import type {
  GatewayAction,
  GatewayProbe,
} from "@openshift-online/hypershell-gateway-management-ui";
import type { ProbeDeliveryFailure } from "@openshift-online/hypershell-domain-probes/fan-out";
import { SpanStatusCode, type MeterProvider } from "@opentelemetry/api";
import { ExportResultCode } from "@opentelemetry/core";
import { OTLPTraceExporter } from "@opentelemetry/exporter-trace-otlp-http";
import {
  AlwaysOffSampler,
  AlwaysOnSampler,
  BasicTracerProvider,
  BatchSpanProcessor,
  InMemorySpanExporter,
  ParentBasedSampler,
  SimpleSpanProcessor,
  type BufferConfig,
  type ReadableSpan,
  type Sampler,
  type SpanExporter,
} from "@opentelemetry/sdk-trace-base";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  RootTraceIdGenerator,
  backstopExporter,
  createGatewayTraceSink,
  createGatewayTracing,
  deliveryHealthMeterProvider,
} from "./gateway-trace-sink";

/** An exporter that accepts a batch but never acknowledges it. */
const blockingExporter: SpanExporter = {
  export: () => undefined,
  forceFlush: () => Promise.resolve(),
  shutdown: () => Promise.resolve(),
};

const traceId = "0af7651916cd43dd8448eb211c80319c";
const correlationId = "correlation-1";

function probe(
  name: GatewayProbe["name"],
  overrides: Partial<{
    action: GatewayAction;
    failureKind: GatewayProbe["fields"]["failureKind"];
    operationId: string;
    outcome: GatewayProbe["fields"]["outcome"];
    traceId: string | undefined;
  }> = {},
): GatewayProbe {
  const action = overrides.action ?? "list";
  return Object.freeze({
    context: Object.freeze({
      correlationId,
      ...(overrides.operationId === undefined
        ? {}
        : { operationId: overrides.operationId }),
      ...("traceId" in overrides
        ? { traceId: overrides.traceId }
        : { traceId }),
    }),
    fields: Object.freeze({
      action,
      failureKind: overrides.failureKind ?? null,
      outcome: overrides.outcome ?? "started",
    }),
    name,
    occurredAt: "2026-08-06T18:00:00.000Z",
    schemaVersion: 1,
  });
}

function testTracer(rootSampler: Sampler = new AlwaysOnSampler()) {
  const exporter = new InMemorySpanExporter();
  const idGenerator = new RootTraceIdGenerator();
  const provider = new BasicTracerProvider({
    idGenerator,
    sampler: new ParentBasedSampler({ root: rootSampler }),
    spanProcessors: [new SimpleSpanProcessor(exporter)],
  });
  return { exporter, idGenerator, tracer: provider.getTracer("test") };
}

function byName(spans: readonly ReadableSpan[], name: string): ReadableSpan {
  const span = spans.find((candidate) => candidate.name === name);
  if (span === undefined) {
    throw new Error(`no finished span named ${name}`);
  }
  return span;
}

describe("gateway trace sink", () => {
  let exporter: InMemorySpanExporter;
  let sink: ReturnType<typeof createGatewayTraceSink>;

  beforeEach(() => {
    const harness = testTracer();
    exporter = harness.exporter;
    sink = createGatewayTraceSink(harness.tracer, {
      beginTrace: (id) => {
        harness.idGenerator.primeTraceId(id);
      },
    });
  });

  it("adopts the probe trace id and nests the dependency under the workflow", () => {
    sink.sink.publish(probe("gateway.workflow.started"));
    sink.sink.publish(probe("gateway.dependency.attempted"));
    sink.sink.publish(
      probe("gateway.dependency.completed", { outcome: "succeeded" }),
    );
    sink.sink.publish(
      probe("gateway.workflow.completed", { outcome: "succeeded" }),
    );

    const finished = exporter.getFinishedSpans();
    const workflow = byName(finished, "gateway.workflow.list");
    const dependency = byName(finished, "gateway.dependency.list");

    expect(workflow.spanContext().traceId).toBe(traceId);
    expect(dependency.spanContext().traceId).toBe(traceId);
    // The workflow span is a true trace root: it adopts the chosen trace id yet
    // has no parent, so the trace is never decapitated by a synthetic remote
    // parent that no service exports. The dependency nests under the workflow.
    expect(workflow.parentSpanContext).toBeUndefined();
    expect(dependency.parentSpanContext?.spanId).toBe(
      workflow.spanContext().spanId,
    );
    expect(workflow.status.code).toBe(SpanStatusCode.OK);
    expect(workflow.attributes["gateway.action"]).toBe("list");
    expect(workflow.attributes["gateway.outcome"]).toBe("succeeded");
    // The dependency span closes before the workflow span.
    expect(finished.map((span) => span.name)).toEqual([
      "gateway.dependency.list",
      "gateway.workflow.list",
    ]);
  });

  it("marks a failed workflow and retains the operation identifier", () => {
    sink.sink.publish(probe("gateway.workflow.started", { action: "rename" }));
    sink.sink.publish(
      probe("gateway.dependency.attempted", { action: "rename" }),
    );
    sink.sink.publish(
      probe("gateway.dependency.completed", {
        action: "rename",
        failureKind: "conflict",
        operationId: "operation-1",
        outcome: "conflicted",
      }),
    );
    sink.sink.publish(
      probe("gateway.workflow.completed", {
        action: "rename",
        failureKind: "conflict",
        operationId: "operation-1",
        outcome: "conflicted",
      }),
    );

    const finished = exporter.getFinishedSpans();
    for (const span of finished) {
      expect(span.status.code).toBe(SpanStatusCode.ERROR);
      expect(span.attributes["gateway.failure_kind"]).toBe("conflict");
      expect(span.attributes["gateway.outcome"]).toBe("conflicted");
      expect(span.attributes["hypershell.operation_id"]).toBe("operation-1");
    }
  });

  it("renders the active dependency span as a W3C traceparent", () => {
    sink.sink.publish(probe("gateway.workflow.started"));
    sink.sink.publish(probe("gateway.dependency.attempted"));

    const active = sink.traceParentFor(correlationId);

    expect(active?.traceparent).toMatch(
      new RegExp(`^00-${traceId}-[0-9a-f]{16}-01$`),
    );
    expect(active?.tracestate).toBeUndefined();
  });

  it("reports no trace context once the workflow completes", () => {
    sink.sink.publish(probe("gateway.workflow.started"));
    sink.sink.publish(probe("gateway.dependency.attempted"));
    sink.sink.publish(
      probe("gateway.dependency.completed", { outcome: "succeeded" }),
    );
    sink.sink.publish(
      probe("gateway.workflow.completed", { outcome: "succeeded" }),
    );

    expect(sink.traceParentFor(correlationId)).toBeUndefined();
  });

  it("ignores dependency and completion probes with no started workflow", () => {
    expect(() => {
      sink.sink.publish(probe("gateway.dependency.attempted"));
      sink.sink.publish(
        probe("gateway.dependency.completed", { outcome: "succeeded" }),
      );
      sink.sink.publish(
        probe("gateway.workflow.completed", { outcome: "succeeded" }),
      );
    }).not.toThrow();
    expect(exporter.getFinishedSpans()).toHaveLength(0);
  });

  it("drops a trace when the sampler declines it", () => {
    const harness = testTracer(new AlwaysOffSampler());
    const unsampled = createGatewayTraceSink(harness.tracer, {
      beginTrace: (id) => {
        harness.idGenerator.primeTraceId(id);
      },
    });

    unsampled.sink.publish(probe("gateway.workflow.started"));
    unsampled.sink.publish(probe("gateway.dependency.attempted"));
    const active = unsampled.traceParentFor(correlationId);
    unsampled.sink.publish(
      probe("gateway.dependency.completed", { outcome: "succeeded" }),
    );
    unsampled.sink.publish(
      probe("gateway.workflow.completed", { outcome: "succeeded" }),
    );

    // A declined trace still propagates its ids, with the sampled flag cleared,
    // so downstream services make the same decision.
    expect(active?.traceparent).toMatch(
      new RegExp(`^00-${traceId}-[0-9a-f]{16}-00$`),
    );
    expect(harness.exporter.getFinishedSpans()).toHaveLength(0);
  });

  it("keeps high-cardinality and sensitive values out of every exported span", () => {
    // A correlation identifier is opaque and high-cardinality; treat it as a
    // stand-in for any secret that must never reach the collector.
    const secret = "corr-Bearer-eyJhbGciOi-SEEDED-SECRET";
    const withSecret = (base: GatewayProbe): GatewayProbe =>
      Object.freeze({
        ...base,
        context: Object.freeze({ ...base.context, correlationId: secret }),
      });

    sink.sink.publish(withSecret(probe("gateway.workflow.started")));
    sink.sink.publish(withSecret(probe("gateway.dependency.attempted")));
    sink.sink.publish(
      withSecret(
        probe("gateway.dependency.completed", { outcome: "succeeded" }),
      ),
    );
    sink.sink.publish(
      withSecret(
        probe("gateway.workflow.completed", {
          operationId: "operation-1",
          outcome: "succeeded",
        }),
      ),
    );

    const allowedKeys = new Set([
      "gateway.action",
      "gateway.outcome",
      "gateway.failure_kind",
      "hypershell.operation_id",
    ]);
    const finished = exporter.getFinishedSpans();
    expect(finished).toHaveLength(2);
    for (const span of finished) {
      // Span names come from a bounded template, never a raw identifier.
      expect(span.name).toMatch(/^gateway\.(workflow|dependency)\.[a-z]+$/);
      for (const key of Object.keys(span.attributes)) {
        expect(allowedKeys.has(key)).toBe(true);
      }
      // The secret appears in no span name or attribute value.
      const serialized = JSON.stringify({
        attributes: span.attributes,
        name: span.name,
      });
      expect(serialized).not.toContain(secret);
      expect(serialized).not.toContain("SEEDED-SECRET");
      expect(serialized).not.toContain("Bearer");
    }
  });
});

describe("createGatewayTracing flush on page hide", () => {
  const config = {
    serviceName: "hypershell-web-console",
    tracesEndpoint: "http://localhost/telemetry/v1/traces",
  };

  function setVisibility(state: "hidden" | "visible"): void {
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      get: () => state,
    });
  }

  afterEach(() => {
    vi.restoreAllMocks();
    setVisibility("visible");
  });

  it("flushes buffered spans on hidden visibilitychange and on pagehide", async () => {
    const flush = vi
      .spyOn(BasicTracerProvider.prototype, "forceFlush")
      .mockResolvedValue();
    const tracing = createGatewayTracing(config);

    // A visible transition must not flush; only a hide is a last-chance export.
    setVisibility("visible");
    document.dispatchEvent(new Event("visibilitychange"));
    expect(flush).not.toHaveBeenCalled();

    setVisibility("hidden");
    document.dispatchEvent(new Event("visibilitychange"));
    expect(flush).toHaveBeenCalledTimes(1);

    window.dispatchEvent(new Event("pagehide"));
    expect(flush).toHaveBeenCalledTimes(2);

    vi.spyOn(BasicTracerProvider.prototype, "shutdown").mockResolvedValue();
    await tracing.shutdown();
  });

  it("stops flushing once shutdown removes the listeners", async () => {
    const flush = vi
      .spyOn(BasicTracerProvider.prototype, "forceFlush")
      .mockResolvedValue();
    const shutdown = vi
      .spyOn(BasicTracerProvider.prototype, "shutdown")
      .mockResolvedValue();
    const tracing = createGatewayTracing(config);

    await tracing.shutdown();
    expect(shutdown).toHaveBeenCalledTimes(1);
    flush.mockClear();

    setVisibility("hidden");
    document.dispatchEvent(new Event("visibilitychange"));
    window.dispatchEvent(new Event("pagehide"));
    expect(flush).not.toHaveBeenCalled();
  });
});

describe("createGatewayTracing delivery health", () => {
  const config = {
    serviceName: "hypershell-web-console",
    tracesEndpoint: "http://localhost/telemetry/v1/traces",
  };

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("reports a failed span export as an out-of-band delivery failure", async () => {
    // The collector is unreachable: the exporter yields a FAILED result, which
    // the batch processor would otherwise swallow into its global error handler.
    vi.spyOn(OTLPTraceExporter.prototype, "export").mockImplementation(
      (_spans, resultCallback) => {
        resultCallback({
          code: ExportResultCode.FAILED,
          error: new Error("collector unreachable"),
        });
      },
    );
    const failures: Readonly<ProbeDeliveryFailure>[] = [];
    const tracing = createGatewayTracing(config, {
      reportDeliveryFailure: (failure) => failures.push(failure),
    });

    tracing.sink.publish(probe("gateway.workflow.started"));
    tracing.sink.publish(
      probe("gateway.workflow.completed", { outcome: "succeeded" }),
    );
    // The flush rejects on the failed export; the failure is recorded before the
    // rejection propagates, so settling it here is enough.
    await tracing.forceFlush().catch(() => undefined);

    expect(failures).toEqual([
      {
        errorType: "Error",
        probeName: "gateway.trace.export",
        schemaVersion: 0,
        sinkId: "gateway-trace",
      },
    ]);

    vi.spyOn(OTLPTraceExporter.prototype, "shutdown").mockResolvedValue();
    await tracing.shutdown();
  });

  it("does not report when the export succeeds", async () => {
    vi.spyOn(OTLPTraceExporter.prototype, "export").mockImplementation(
      (_spans, resultCallback) => {
        resultCallback({ code: ExportResultCode.SUCCESS });
      },
    );
    const failures: Readonly<ProbeDeliveryFailure>[] = [];
    const tracing = createGatewayTracing(config, {
      reportDeliveryFailure: (failure) => failures.push(failure),
    });

    tracing.sink.publish(probe("gateway.workflow.started"));
    tracing.sink.publish(
      probe("gateway.workflow.completed", { outcome: "succeeded" }),
    );
    await tracing.forceFlush();

    expect(failures).toEqual([]);

    vi.spyOn(OTLPTraceExporter.prototype, "shutdown").mockResolvedValue();
    await tracing.shutdown();
  });

  it("reports every span dropped by an overflowing queue", () => {
    // A queue that holds one span plus an exporter that never drains it forces
    // the batch processor to drop every subsequent span. Those drops never
    // reach the exporter callback, so only the self-observation meter can
    // surface them.
    vi.useFakeTimers();
    try {
      const failures: Readonly<ProbeDeliveryFailure>[] = [];
      const processorConfig: BufferConfig & {
        selfObsMeterProvider?: MeterProvider;
      } = {
        maxExportBatchSize: 1,
        maxQueueSize: 1,
        scheduledDelayMillis: 1,
        selfObsMeterProvider: deliveryHealthMeterProvider((failure) =>
          failures.push(failure),
        ),
      };
      const provider = new BasicTracerProvider({
        sampler: new AlwaysOnSampler(),
        spanProcessors: [
          new BatchSpanProcessor(blockingExporter, processorConfig),
        ],
      });
      const tracer = provider.getTracer("overflow-test");

      // The exporter never drains, so once the in-flight batch and the single
      // queue slot are taken, every further span overflows and is dropped.
      for (let index = 0; index < 8; index += 1) {
        tracer.startSpan(`span-${String(index)}`).end();
      }

      expect(failures.length).toBeGreaterThanOrEqual(1);
      expect(
        failures.every((failure) => failure.errorType === "queue_full"),
      ).toBe(true);
      expect(failures).toContainEqual({
        errorType: "queue_full",
        probeName: "gateway.trace.export",
        schemaVersion: 0,
        sinkId: "gateway-trace",
      });
    } finally {
      vi.useRealTimers();
    }
  });

  it("reports a wedged exporter that never acknowledges a batch", async () => {
    // The exporter accepts the batch and never calls back. The processor's own
    // export timeout would reject the flush without accounting the loss; the
    // backstop converts the stall into a FAILED result the meter records.
    vi.useFakeTimers();
    try {
      const failures: Readonly<ProbeDeliveryFailure>[] = [];
      const processorConfig: BufferConfig & {
        selfObsMeterProvider?: MeterProvider;
      } = {
        maxExportBatchSize: 1,
        scheduledDelayMillis: 1,
        selfObsMeterProvider: deliveryHealthMeterProvider((failure) =>
          failures.push(failure),
        ),
      };
      const provider = new BasicTracerProvider({
        sampler: new AlwaysOnSampler(),
        spanProcessors: [
          new BatchSpanProcessor(
            backstopExporter(blockingExporter, 15_000),
            processorConfig,
          ),
        ],
      });
      const tracer = provider.getTracer("blocking-test");
      tracer.startSpan("wedged").end();

      const flushed = provider.forceFlush().catch(() => undefined);
      await vi.advanceTimersByTimeAsync(15_000);
      await flushed;

      expect(failures).toEqual([
        {
          errorType: "SpanExportTimeout",
          probeName: "gateway.trace.export",
          schemaVersion: 0,
          sinkId: "gateway-trace",
        },
      ]);
    } finally {
      vi.useRealTimers();
    }
  });

  it("reports a synchronous exporter throw as a delivery failure", async () => {
    // The exporter throws instead of calling back. Without the backstop's throw
    // guard the loss would bypass settlement and go entirely unaccounted.
    const throwingExporter: SpanExporter = {
      export: () => {
        const error = new Error("exporter blew up");
        error.name = "SyncExportError";
        throw error;
      },
      forceFlush: () => Promise.resolve(),
      shutdown: () => Promise.resolve(),
    };
    const failures: Readonly<ProbeDeliveryFailure>[] = [];
    const processorConfig: BufferConfig & {
      selfObsMeterProvider?: MeterProvider;
    } = {
      maxExportBatchSize: 1,
      scheduledDelayMillis: 1,
      selfObsMeterProvider: deliveryHealthMeterProvider((failure) =>
        failures.push(failure),
      ),
    };
    const provider = new BasicTracerProvider({
      sampler: new AlwaysOnSampler(),
      spanProcessors: [
        new BatchSpanProcessor(
          backstopExporter(throwingExporter, 15_000),
          processorConfig,
        ),
      ],
    });
    const tracer = provider.getTracer("throwing-test");
    tracer.startSpan("thrown").end();

    await provider.forceFlush().catch(() => undefined);

    expect(failures).toEqual([
      {
        errorType: "SyncExportError",
        probeName: "gateway.trace.export",
        schemaVersion: 0,
        sinkId: "gateway-trace",
      },
    ]);
  });
});
