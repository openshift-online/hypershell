import {
  ROOT_CONTEXT,
  SpanKind,
  SpanStatusCode,
  TraceFlags,
  isSpanContextValid,
  trace as otelTrace,
  type Span,
  type SpanContext,
  type Tracer,
} from "@opentelemetry/api";
import {
  BasicTracerProvider,
  BatchSpanProcessor,
  ParentBasedSampler,
  TraceIdRatioBasedSampler,
} from "@opentelemetry/sdk-trace-base";
import { OTLPTraceExporter } from "@opentelemetry/exporter-trace-otlp-http";
import { resourceFromAttributes } from "@opentelemetry/resources";
import { ATTR_SERVICE_NAME } from "@opentelemetry/semantic-conventions";

import type { TracingConfig } from "../../config.js";
import {
  disabledTracing,
  type BffTracing,
  type ProxyOutcome,
  type ProxySpan,
  type StartProxySpanInput,
  type TelemetryIngestResult,
  type UpstreamTraceContext,
} from "../../tracing.js";

const traceparentPattern =
  /^00-(?<traceId>[0-9a-f]{32})-(?<spanId>[0-9a-f]{16})-(?<flags>[0-9a-f]{2})$/u;
// A permissive W3C `tracestate`: comma-separated members, bounded length. The
// value is untrusted and only forwarded, never parsed, so a light structural
// check is enough to drop obvious garbage before propagation.
const tracestatePattern = /^[ \t]*[!-~]+=[ -~]*(,[ \t]*[!-~]+=[ -~]*){0,31}$/u;
const zeroTraceId = "0".repeat(32);
const zeroSpanId = "0".repeat(16);
const ingestTimeoutMs = 5_000;

const versionSegment = /^v\d+$/u;

/**
 * A path segment is a resource id (not a collection or action name) when it
 * carries a digit or is long. That is true of every ULID, UUID, or numeric id
 * the API mints and of none of the fixed collection or action names, so
 * collapsing it keeps a raw identifier out of the route template.
 */
function isIdSegment(segment: string): boolean {
  return /\d/u.test(segment) || segment.length >= 20;
}

/**
 * Collapses resource ids in a request path to a bounded route template, so the
 * span name and `http.route` stay low-cardinality (WEB-TRACE-07). The API is a
 * flat REST surface under `/api/<group>/v<n>/<collection>[/{id}...]`; after the
 * version segment, id segments collapse to `{id}` while collection and action
 * segments stay literal. A path with no version segment collapses every
 * id-shaped segment, so an unexpected shape can never blow up cardinality.
 */
export function routeTemplateFrom(path: string): string {
  const segments = path.split("/").filter((segment) => segment.length > 0);
  const versionIndex = segments.findIndex((segment) =>
    versionSegment.test(segment),
  );
  const template = segments.map((segment, index) =>
    index > versionIndex && isIdSegment(segment) ? "{id}" : segment,
  );
  return `/${template.join("/")}`;
}

/** Parses a W3C `traceparent`, returning a remote span context or `undefined`. */
function parseTraceparent(value: string): SpanContext | undefined {
  const groups = traceparentPattern.exec(value)?.groups;
  if (
    groups?.flags === undefined ||
    groups.spanId === undefined ||
    groups.traceId === undefined
  ) {
    return undefined;
  }
  const { flags, spanId, traceId } = groups;
  if (traceId === zeroTraceId || spanId === zeroSpanId) {
    return undefined;
  }
  return {
    isRemote: true,
    spanId,
    traceFlags: Number.parseInt(flags, 16),
    traceId,
  };
}

function isValidTracestate(value: string | undefined): value is string {
  return (
    value !== undefined && value.length <= 512 && tracestatePattern.test(value)
  );
}

function upstreamContext(
  span: Span,
  forwardedTracestate: string | undefined,
): UpstreamTraceContext | undefined {
  const spanContext = span.spanContext();
  if (!isSpanContextValid(spanContext)) {
    return undefined;
  }
  const flags =
    (spanContext.traceFlags & TraceFlags.SAMPLED) === 0 ? "00" : "01";
  const traceparent = `00-${spanContext.traceId}-${spanContext.spanId}-${flags}`;
  return forwardedTracestate === undefined
    ? { traceparent }
    : { traceparent, tracestate: forwardedTracestate };
}

function spanStatusFor(outcome: ProxyOutcome): SpanStatusCode {
  return outcome === "server_error" || outcome === "timeout"
    ? SpanStatusCode.ERROR
    : SpanStatusCode.OK;
}

// Bounds on the OTLP/HTTP JSON envelope. The Fastify body limit already caps the
// raw size; these caps additionally bound the structural walk and reject a
// payload whose arrays are implausibly large before it is relayed.
const maxResourceSpans = 10_000;
const maxScopeSpans = 10_000;
const maxSpans = 100_000;

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isBoundedObjectArray(value: unknown, limit: number): boolean {
  return Array.isArray(value) && value.length <= limit && value.every(isObject);
}

/**
 * Validates the OTLP/HTTP trace envelope structurally and within bounds:
 * `resourceSpans` is an array of objects, each optional `scopeSpans` is an array
 * of objects, and each optional `spans` is an array of objects, all under a
 * fixed cap. This rejects a body that is not well-formed OTLP before it reaches
 * the collector, rather than accepting anything with a `resourceSpans` array and
 * letting the collector reject it after the browser was told the export was
 * accepted (WEB-TRACE-02).
 */
function isOtlpTracePayload(payload: unknown): boolean {
  if (!isObject(payload)) {
    return false;
  }
  if (!isBoundedObjectArray(payload.resourceSpans, maxResourceSpans)) {
    return false;
  }
  for (const resourceSpan of payload.resourceSpans as Record<
    string,
    unknown
  >[]) {
    const { scopeSpans } = resourceSpan;
    if (
      scopeSpans !== undefined &&
      !isBoundedObjectArray(scopeSpans, maxScopeSpans)
    ) {
      return false;
    }
    for (const scopeSpan of (scopeSpans ?? []) as Record<string, unknown>[]) {
      const { spans } = scopeSpan;
      if (spans !== undefined && !isBoundedObjectArray(spans, maxSpans)) {
        return false;
      }
    }
  }
  return true;
}

/**
 * Builds the BFF tracing adapter. Spans are managed explicitly per request and
 * exported over OTLP/HTTP; no global tracer or context manager is registered.
 * Returns the disabled port when no collector is configured, so a deployment
 * without tracing starts normally.
 */
export function createBffTracing(
  config: TracingConfig | undefined,
): BffTracing {
  if (config === undefined) {
    return disabledTracing;
  }
  const tracing = config;

  const exporter = new OTLPTraceExporter({ url: tracing.tracesEndpoint });
  const provider = new BasicTracerProvider({
    resource: resourceFromAttributes({
      [ATTR_SERVICE_NAME]: tracing.serviceName,
    }),
    sampler: new ParentBasedSampler({
      root: new TraceIdRatioBasedSampler(tracing.sampleRatio),
    }),
    spanProcessors: [new BatchSpanProcessor(exporter)],
  });
  const tracer: Tracer = provider.getTracer("hypershell-web-console-bff");

  function startProxySpan(input: StartProxySpanInput): ProxySpan {
    const parent =
      input.traceparent === undefined
        ? undefined
        : parseTraceparent(input.traceparent);
    const continued = parent !== undefined && isSpanContextValid(parent);
    const parentContext = continued
      ? otelTrace.setSpanContext(ROOT_CONTEXT, parent)
      : ROOT_CONTEXT;
    // A `tracestate` is forwarded only alongside a valid inbound `traceparent`;
    // a state without a parent trace has nothing to continue.
    const forwardedTracestate =
      continued && isValidTracestate(input.tracestate)
        ? input.tracestate
        : undefined;
    // Name the span by method and the bounded route template (for example
    // "GET /api/hypershell/v1/gateways/{id}") so Jaeger groups by endpoint
    // rather than collapsing every proxied call onto one wildcard operation.
    const routeTemplate = routeTemplateFrom(input.path);
    const span = tracer.startSpan(
      `${input.method} ${routeTemplate}`,
      {
        attributes: {
          "http.request.method": input.method,
          "http.route": routeTemplate,
          "hypershell.correlation_id": input.correlationId,
        },
        kind: SpanKind.SERVER,
      },
      parentContext,
    );

    return {
      end(outcome, statusCode) {
        span.setAttribute("http.response.status_code", statusCode);
        span.setAttribute("hypershell.outcome", outcome);
        span.setStatus({ code: spanStatusFor(outcome) });
        span.end();
      },
      upstream: () => upstreamContext(span, forwardedTracestate),
    };
  }

  async function ingestTraces(
    payload: unknown,
  ): Promise<TelemetryIngestResult> {
    if (!isOtlpTracePayload(payload)) {
      return "rejected";
    }
    try {
      const response = await fetch(tracing.tracesEndpoint, {
        body: JSON.stringify(payload),
        headers: { "content-type": "application/json" },
        method: "POST",
        signal: AbortSignal.timeout(ingestTimeoutMs),
      });
      if (response.ok) {
        return "accepted";
      }
      // A collector 4xx means the collector rejected the payload as malformed on
      // its stricter parse; surface it as a rejection so the browser learns its
      // telemetry was bad rather than seeing a 202. Transient 408/429 and every
      // 5xx are best-effort unavailability, never surfaced as a client error.
      if (
        response.status >= 400 &&
        response.status < 500 &&
        response.status !== 408 &&
        response.status !== 429
      ) {
        return "rejected";
      }
      return "unavailable";
    } catch {
      // Best-effort: an unreachable collector never fails the browser request.
      return "unavailable";
    }
  }

  return {
    enabled: true,
    ingestTraces,
    shutdown: () => provider.shutdown(),
    startProxySpan,
  };
}
