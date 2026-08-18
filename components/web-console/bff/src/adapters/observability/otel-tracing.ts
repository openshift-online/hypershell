import {
  ROOT_CONTEXT,
  SpanKind,
  SpanStatusCode,
  defaultTextMapGetter,
  defaultTextMapSetter,
  isSpanContextValid,
  trace as otelTrace,
  type Context,
  type Span,
  type Tracer,
} from "@opentelemetry/api";
import { W3CTraceContextPropagator } from "@opentelemetry/core";
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

const ingestTimeoutMs = 5_000;

// The conformant W3C propagator handles extraction and injection: it accepts
// current and future `traceparent` versions, rejects an all-zero or malformed
// one, and drops a malformed `tracestate` while still continuing a trace whose
// `traceparent` is valid. It is stateless, so a single instance is shared.
const propagator = new W3CTraceContextPropagator();

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

/**
 * Extracts the inbound W3C trace context. An absent, malformed, all-zero, or
 * unsupported-version `traceparent` yields the root context, so a fresh trace is
 * started; a valid one continues the trace and carries a valid `tracestate`
 * forward. A `tracestate` without a valid `traceparent` is discarded because the
 * root context it lands in has no trace to continue.
 */
function parentContextFrom(input: StartProxySpanInput): Context {
  if (input.traceparent === undefined) {
    return ROOT_CONTEXT;
  }
  const carrier: Record<string, string> = { traceparent: input.traceparent };
  if (input.tracestate !== undefined) {
    carrier.tracestate = input.tracestate;
  }
  return propagator.extract(ROOT_CONTEXT, carrier, defaultTextMapGetter);
}

/**
 * Serializes the BFF span's own context into upstream `traceparent`/`tracestate`
 * headers via the propagator, so a continued trace forwards the inherited
 * `tracestate` and the sampled flag reflects the span's decision. Returns
 * `undefined` when the span has no valid context. The propagator emits an empty
 * string for an absent trace state, which is not a header worth forwarding, so
 * it is normalized to no `tracestate`.
 */
function upstreamContextFor(span: Span): UpstreamTraceContext | undefined {
  if (!isSpanContextValid(span.spanContext())) {
    return undefined;
  }
  const carrier: Record<string, string> = {};
  propagator.inject(
    otelTrace.setSpan(ROOT_CONTEXT, span),
    carrier,
    defaultTextMapSetter,
  );
  const { traceparent, tracestate } = carrier;
  if (traceparent === undefined) {
    return undefined;
  }
  return tracestate === undefined || tracestate === ""
    ? { traceparent }
    : { traceparent, tracestate };
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
    const parentContext = parentContextFrom(input);
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
      upstream: () => upstreamContextFor(span),
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
