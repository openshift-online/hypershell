import type { GatewayProbe } from "@openshift-online/hypershell-gateway-management-ui";
import type { DomainProbeSink } from "@openshift-online/hypershell-domain-probes/fan-out";
import {
  ROOT_CONTEXT,
  SpanKind,
  SpanStatusCode,
  TraceFlags,
  context as otelContext,
  isSpanContextValid,
  trace as otelTrace,
  type Context,
  type Span,
  type SpanContext,
  type Tracer,
} from "@opentelemetry/api";
import {
  BasicTracerProvider,
  BatchSpanProcessor,
  ParentBasedSampler,
  AlwaysOnSampler,
  RandomIdGenerator,
} from "@opentelemetry/sdk-trace-base";
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
   * Decides whether one trace is recorded, keyed by its trace id so the
   * decision is stable for the whole trace. Defaults to always sampling.
   */
  isSampled?: (traceId: string) => boolean;
  /** Generates the 16-byte span id (32 hex) of the manufactured remote parent. */
  generateSpanId?: () => string;
}

export interface GatewayTracingConfig {
  serviceName: string;
  /** Same-origin OTLP/HTTP traces path the browser exporter posts to. */
  tracesEndpoint: string;
  /** Fraction of traces to record, 0..1. Defaults to 1 (record all). */
  sampleRatio?: number;
}

const sinkId = "gateway-trace";
const tracerName = "gateway-trace-sink";

interface SpanEntry {
  workflow: Span;
  dependency?: Span;
}

/**
 * Manufactures the remote parent context that makes a workflow span adopt the
 * caller-chosen trace id. The parent is marked sampled only when `isSampled`
 * accepts the trace id; a `ParentBasedSampler` then honours that decision for
 * the whole trace, so the W3C sampled flag stays consistent end to end.
 */
function remoteParentContext(
  traceId: string,
  spanId: string,
  sampled: boolean,
): Context {
  const spanContext: SpanContext = {
    isRemote: true,
    spanId,
    traceFlags: sampled ? TraceFlags.SAMPLED : TraceFlags.NONE,
    traceId,
  };
  return otelTrace.setSpanContext(ROOT_CONTEXT, spanContext);
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
 * probes onto OpenTelemetry spans. The sink adopts the trace id carried on each
 * probe context so a workflow span joins the same trace the browser propagates
 * to the BFF and API. Span names are drawn from a bounded action template so
 * cardinality stays fixed.
 */
export function createGatewayTraceSink(
  tracer: Tracer,
  options: GatewayTraceSinkOptions = {},
): GatewayTraceSink {
  const generateSpanId =
    options.generateSpanId ?? (() => new RandomIdGenerator().generateSpanId());
  const isSampled = options.isSampled ?? (() => true);
  const spansByCorrelation = new Map<string, SpanEntry>();

  function startWorkflow(probe: GatewayProbe): void {
    const { correlationId, traceId } = probe.context;
    const parent =
      traceId === undefined
        ? otelContext.active()
        : remoteParentContext(traceId, generateSpanId(), isSampled(traceId));
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
    const flags = (spanContext.traceFlags & TraceFlags.SAMPLED) === 0
      ? "00"
      : "01";
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
 * needed.
 * The batch processor flushes on document hide, so page unload does not lose
 * buffered spans.
 */
export function createGatewayTracing(
  config: GatewayTracingConfig,
): GatewayTracing {
  const ratio = config.sampleRatio ?? 1;
  const exporter = new OTLPTraceExporter({ url: config.tracesEndpoint });
  const provider = new BasicTracerProvider({
    resource: resourceFromAttributes({
      [ATTR_SERVICE_NAME]: config.serviceName,
    }),
    sampler: new ParentBasedSampler({ root: new AlwaysOnSampler() }),
    spanProcessors: [new BatchSpanProcessor(exporter)],
  });
  const tracer = provider.getTracer(tracerName);
  const { sink, traceParentFor } = createGatewayTraceSink(tracer, {
    isSampled: (traceId) => sampledByRatio(traceId, ratio),
  });
  return {
    forceFlush: () => provider.forceFlush(),
    shutdown: () => provider.shutdown(),
    sink,
    traceParentFor,
  };
}

/**
 * Deterministic per-trace sampling decision. The leading 4 bytes of the trace
 * id map to a value in [0, 1); a trace records when that value is below the
 * configured ratio. Deterministic keying keeps the browser, BFF, and API in
 * agreement without sharing a decision.
 */
function sampledByRatio(traceId: string, ratio: number): boolean {
  if (ratio >= 1) {
    return true;
  }
  if (ratio <= 0) {
    return false;
  }
  const bucket = Number.parseInt(traceId.slice(0, 8), 16);
  return bucket / 0x1_0000_0000 < ratio;
}
