import type { GatewayProbe } from "@openshift-online/hypershell-gateway-management-ui";
import type {
  DomainProbeSink,
  ProbeDeliveryFailure,
} from "@openshift-online/hypershell-domain-probes/fan-out";
import {
  ROOT_CONTEXT,
  SpanKind,
  SpanStatusCode,
  TraceFlags,
  context as otelContext,
  isSpanContextValid,
  trace as otelTrace,
  type Span,
  type Tracer,
} from "@opentelemetry/api";
import {
  BasicTracerProvider,
  BatchSpanProcessor,
  ParentBasedSampler,
  RandomIdGenerator,
  TraceIdRatioBasedSampler,
  type IdGenerator,
  type SpanExporter,
} from "@opentelemetry/sdk-trace-base";
import { ExportResultCode } from "@opentelemetry/core";
import { OTLPTraceExporter } from "@opentelemetry/exporter-trace-otlp-http";
import { resourceFromAttributes } from "@opentelemetry/resources";
import { ATTR_SERVICE_NAME } from "@opentelemetry/semantic-conventions";

/** W3C `traceparent`/`tracestate` header pair for outbound propagation. */
export interface GatewayTraceContext {
  traceparent: string;
  tracestate?: string;
}

/** A gateway trace sink plus the propagation reader that feeds the API client. */
export interface GatewayTraceSink {
  /**
   * Reads the active workflow (or in-flight dependency) span for one
   * correlation identifier and renders its W3C context, or `undefined` when no
   * span is active for that correlation identifier.
   */
  traceParentFor: (correlationId: string) => GatewayTraceContext | undefined;
  sink: DomainProbeSink<GatewayProbe>;
}

/** A gateway trace sink together with its provider lifecycle controls. */
export interface GatewayTracing extends GatewayTraceSink {
  forceFlush(): Promise<void>;
  shutdown(): Promise<void>;
}

export interface GatewayTraceSinkOptions {
  /**
   * Primes the trace id that the next root workflow span adopts, so the
   * workflow span is a true root that owns the app-chosen trace id rather than
   * descending from a synthetic remote parent. Wired to the provider's
   * {@link RootTraceIdGenerator}. When omitted, a workflow keeps its probe
   * trace id only for propagation and the exported root uses a generated id.
   */
  beginTrace?: (traceId: string) => void;
}

export interface GatewayTracingConfig {
  serviceName: string;
  /** Same-origin OTLP/HTTP traces path the browser exporter posts to. */
  tracesEndpoint: string;
  /** Fraction of traces to record, 0..1. Defaults to 1 (record all). */
  sampleRatio?: number;
}

export interface GatewayTracingOptions {
  /**
   * Records a span delivery failure that surfaces after buffering, when the
   * batch exporter cannot reach the collector. Span export is asynchronous, so
   * a failed batch would otherwise be dropped silently; routing it here makes
   * the loss observable through the domain probe delivery-health accounting.
   */
  reportDeliveryFailure?: (failure: Readonly<ProbeDeliveryFailure>) => void;
}

const sinkId = "gateway-trace";
const tracerName = "gateway-trace-sink";
// Synthetic probe name for a failure that is not tied to one probe but to the
// asynchronous export of a batch of spans this sink already accepted.
const traceExportProbeName = "gateway.trace.export";

/**
 * Wraps a span exporter so every failed export is reported as a delivery
 * failure. Both the batch timer and an explicit `forceFlush` funnel through the
 * exporter's result callback, so this catches an unreachable collector on
 * either path exactly once, rather than letting the SDK swallow it into its
 * global error handler.
 */
function reportingExporter(
  inner: SpanExporter,
  report: (failure: Readonly<ProbeDeliveryFailure>) => void,
): SpanExporter {
  return {
    export(spans, resultCallback) {
      inner.export(spans, (result) => {
        if (result.code === ExportResultCode.FAILED) {
          report({
            errorType: result.error?.name ?? "SpanExportError",
            probeName: traceExportProbeName,
            schemaVersion: 0,
            sinkId,
          });
        }
        resultCallback(result);
      });
    },
    forceFlush: () => inner.forceFlush?.() ?? Promise.resolve(),
    shutdown: () => inner.shutdown(),
  };
}

/**
 * Id generator that lets the caller choose the trace id of the next root span
 * while keeping every span id random. A workflow span is the origin of the
 * distributed trace, so it must be a true root; priming the trace id here lets
 * that root still adopt the app-chosen id, joining the trace the browser
 * propagates to the BFF and API without a synthetic remote parent (which would
 * leave the trace decapitated by a parent span that no service ever exports).
 */
export class RootTraceIdGenerator implements IdGenerator {
  private nextTraceId: string | undefined;
  private readonly random = new RandomIdGenerator();

  /** Sets the trace id the next generated root span adopts. */
  primeTraceId(traceId: string): void {
    this.nextTraceId = traceId;
  }

  generateTraceId(): string {
    const chosen = this.nextTraceId;
    this.nextTraceId = undefined;
    return chosen ?? this.random.generateTraceId();
  }

  generateSpanId(): string {
    return this.random.generateSpanId();
  }
}

interface SpanEntry {
  workflow: Span;
  dependency?: Span;
}

/** Terminal outcomes that mark a span failed rather than ok. */
function isFailureOutcome(outcome: GatewayProbe["fields"]["outcome"]): boolean {
  return outcome !== "started" && outcome !== "succeeded";
}

function applyTerminalOutcome(span: Span, probe: GatewayProbe): void {
  const { failureKind, outcome } = probe.fields;
  span.setAttribute("gateway.outcome", outcome);
  if (failureKind !== null) {
    span.setAttribute("gateway.failure_kind", failureKind);
  }
  // The operation identifier is present only on failing terminal probes and is
  // the sole bridge from a failed workflow to its API-side operation record.
  if (probe.context.operationId !== undefined) {
    span.setAttribute("hypershell.operation_id", probe.context.operationId);
  }
  span.setStatus({
    code: isFailureOutcome(outcome) ? SpanStatusCode.ERROR : SpanStatusCode.OK,
  });
}

/**
 * Builds a domain probe sink that projects gateway workflow and dependency
 * probes onto OpenTelemetry spans. A workflow span is a true root that adopts
 * the trace id carried on each probe context (through the primed
 * {@link RootTraceIdGenerator}), so a workflow span joins the same trace the
 * browser propagates to the BFF and API while remaining the origin of that
 * trace. Span names are drawn from a bounded action template so cardinality
 * stays fixed.
 */
export function createGatewayTraceSink(
  tracer: Tracer,
  options: GatewayTraceSinkOptions = {},
): GatewayTraceSink {
  const beginTrace = options.beginTrace ?? ((): void => undefined);
  const spansByCorrelation = new Map<string, SpanEntry>();

  function startWorkflow(probe: GatewayProbe): void {
    const { correlationId, traceId } = probe.context;
    // A workflow with a chosen trace id is the trace root: prime the generator
    // and start it with no parent. Without a chosen id, fall back to the active
    // context so any caller-established parent still nests.
    let parent = otelContext.active();
    if (traceId !== undefined) {
      beginTrace(traceId);
      parent = ROOT_CONTEXT;
    }
    const workflow = tracer.startSpan(
      `gateway.workflow.${probe.fields.action}`,
      {
        attributes: { "gateway.action": probe.fields.action },
        kind: SpanKind.INTERNAL,
      },
      parent,
    );
    spansByCorrelation.set(correlationId, { workflow });
  }

  function startDependency(probe: GatewayProbe): void {
    const entry = spansByCorrelation.get(probe.context.correlationId);
    if (entry === undefined) {
      return;
    }
    const parent = otelTrace.setSpan(otelContext.active(), entry.workflow);
    entry.dependency = tracer.startSpan(
      `gateway.dependency.${probe.fields.action}`,
      {
        attributes: { "gateway.action": probe.fields.action },
        kind: SpanKind.CLIENT,
      },
      parent,
    );
  }

  function completeDependency(probe: GatewayProbe): void {
    const entry = spansByCorrelation.get(probe.context.correlationId);
    if (entry?.dependency === undefined) {
      return;
    }
    applyTerminalOutcome(entry.dependency, probe);
    entry.dependency.end();
    entry.dependency = undefined;
  }

  function completeWorkflow(probe: GatewayProbe): void {
    const entry = spansByCorrelation.get(probe.context.correlationId);
    if (entry === undefined) {
      return;
    }
    // Defend against a dependency span left open by a dropped completion probe.
    if (entry.dependency !== undefined) {
      entry.dependency.end();
    }
    applyTerminalOutcome(entry.workflow, probe);
    entry.workflow.end();
    spansByCorrelation.delete(probe.context.correlationId);
  }

  const sink: DomainProbeSink<GatewayProbe> = {
    id: sinkId,
    publish(probe) {
      switch (probe.name) {
        case "gateway.workflow.started":
          startWorkflow(probe);
          return;
        case "gateway.dependency.attempted":
          startDependency(probe);
          return;
        case "gateway.dependency.completed":
          completeDependency(probe);
          return;
        case "gateway.workflow.completed":
          completeWorkflow(probe);
          return;
      }
    },
  };

  function traceParentFor(
    correlationId: string,
  ): GatewayTraceContext | undefined {
    const entry = spansByCorrelation.get(correlationId);
    const span = entry?.dependency ?? entry?.workflow;
    if (span === undefined) {
      return undefined;
    }
    const spanContext = span.spanContext();
    if (!isSpanContextValid(spanContext)) {
      return undefined;
    }
    const flags =
      (spanContext.traceFlags & TraceFlags.SAMPLED) === 0 ? "00" : "01";
    const traceparent = `00-${spanContext.traceId}-${spanContext.spanId}-${flags}`;
    const tracestate = spanContext.traceState?.serialize();
    return tracestate === undefined || tracestate === ""
      ? { traceparent }
      : { traceparent, tracestate };
  }

  return { sink, traceParentFor };
}

/**
 * Wires a browser tracer provider that batches spans and exports them over
 * same-origin OTLP/HTTP, and returns the gateway trace sink bound to it. The
 * provider is not registered as the global tracer; the sink owns every span
 * explicitly, keyed by correlation identifier, so no implicit context is
 * needed. Sampling is a per-trace decision made once at the workflow root by a
 * `TraceIdRatioBasedSampler` and inherited by child spans, so the browser and
 * the BFF (which uses the same OTel sampler) agree on each trace without
 * sharing a decision.
 *
 * `BatchSpanProcessor` exports on a timer, which a browser can discard when a
 * tab is closed or navigated away, losing the tail of a workflow. The provider
 * therefore forces a flush on `visibilitychange` to hidden and on `pagehide`,
 * the last reliable hooks before unload. `shutdown` removes those listeners.
 */
export function createGatewayTracing(
  config: GatewayTracingConfig,
  options: GatewayTracingOptions = {},
): GatewayTracing {
  const ratio = config.sampleRatio ?? 1;
  const idGenerator = new RootTraceIdGenerator();
  const baseExporter = new OTLPTraceExporter({ url: config.tracesEndpoint });
  const exporter =
    options.reportDeliveryFailure === undefined
      ? baseExporter
      : reportingExporter(baseExporter, options.reportDeliveryFailure);
  const provider = new BasicTracerProvider({
    idGenerator,
    resource: resourceFromAttributes({
      [ATTR_SERVICE_NAME]: config.serviceName,
    }),
    sampler: new ParentBasedSampler({
      root: new TraceIdRatioBasedSampler(ratio),
    }),
    spanProcessors: [new BatchSpanProcessor(exporter)],
  });
  const tracer = provider.getTracer(tracerName);
  const { sink, traceParentFor } = createGatewayTraceSink(tracer, {
    beginTrace: (traceId) => {
      idGenerator.primeTraceId(traceId);
    },
  });

  const flushBufferedSpans = (): void => {
    // A failed export is already recorded through the reporting exporter, so the
    // rejected forceFlush promise only needs to be settled to avoid an
    // unhandled rejection; it is not a second failure to count.
    void provider.forceFlush().catch(() => undefined);
  };
  const flushWhenHidden = (): void => {
    if (document.visibilityState === "hidden") {
      flushBufferedSpans();
    }
  };
  let stopFlushOnHide = (): void => undefined;
  if (typeof document !== "undefined" && typeof window !== "undefined") {
    document.addEventListener("visibilitychange", flushWhenHidden);
    window.addEventListener("pagehide", flushBufferedSpans);
    stopFlushOnHide = () => {
      document.removeEventListener("visibilitychange", flushWhenHidden);
      window.removeEventListener("pagehide", flushBufferedSpans);
    };
  }

  return {
    forceFlush: () => provider.forceFlush(),
    shutdown: async () => {
      stopFlushOnHide();
      await provider.shutdown();
    },
    sink,
    traceParentFor,
  };
}
