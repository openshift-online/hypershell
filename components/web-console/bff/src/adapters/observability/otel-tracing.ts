import {
  ROOT_CONTEXT,
  SpanKind,
  SpanStatusCode,
  createNoopMeter,
  defaultTextMapGetter,
  defaultTextMapSetter,
  isSpanContextValid,
  trace as otelTrace,
  type Attributes,
  type Context,
  type Counter,
  type Meter,
  type MeterProvider,
  type MetricOptions,
  type Span,
  type Tracer,
} from "@opentelemetry/api";
import {
  ExportResultCode,
  W3CTraceContextPropagator,
  type ExportResult,
} from "@opentelemetry/core";
import {
  BasicTracerProvider,
  BatchSpanProcessor,
  ParentBasedSampler,
  TraceIdRatioBasedSampler,
  type BufferConfig,
  type SpanExporter,
} from "@opentelemetry/sdk-trace-base";
import { OTLPTraceExporter } from "@opentelemetry/exporter-trace-otlp-http";
import { resourceFromAttributes } from "@opentelemetry/resources";
import {
  ATTR_ERROR_TYPE,
  ATTR_SERVICE_NAME,
} from "@opentelemetry/semantic-conventions";
import { z } from "zod";

import type { TracingConfig } from "../../config.js";
import {
  disabledTracing,
  type BffDeliveryHealthSnapshot,
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
// raw byte size; these additionally bound the structural walk and reject a
// payload whose arrays are implausibly large before it is relayed.
const maxResourceSpans = 10_000;
const maxScopeSpans = 10_000;
const maxSpans = 100_000;
const maxAttributes = 1_024;
const maxEvents = 1_024;
const maxLinks = 1_024;

// Bounds on the nesting shape itself. A recursive AnyValue nested thousands of
// levels deep would drive the recursive schema past the JavaScript call-stack
// limit and throw a RangeError instead of returning a validation failure. An
// iterative pre-pass caps nesting depth (so the schema recursion stays shallow)
// and total node count before any recursive validation runs.
const maxStructuralDepth = 64;
const maxStructuralNodes = 1_000_000;

/**
 * Verifies the payload's object graph stays within a bounded nesting depth and
 * node count, walking it with an explicit stack so the check itself never
 * recurses. Rejecting an over-deep payload here keeps the recursive AnyValue
 * schema from being driven past the call-stack limit, where it would throw
 * rather than return a rejection.
 */
function withinStructuralBudget(payload: unknown): boolean {
  const stack: { depth: number; node: unknown }[] = [
    { depth: 0, node: payload },
  ];
  let nodes = 0;
  while (stack.length > 0) {
    const entry = stack.pop();
    if (entry === undefined) {
      break;
    }
    nodes += 1;
    if (nodes > maxStructuralNodes || entry.depth > maxStructuralDepth) {
      return false;
    }
    const { depth, node } = entry;
    if (Array.isArray(node)) {
      for (const child of node) {
        stack.push({ depth: depth + 1, node: child });
      }
    } else if (node !== null && typeof node === "object") {
      for (const child of Object.values(node)) {
        stack.push({ depth: depth + 1, node: child });
      }
    }
  }
  return true;
}

// Trace and span ids are hex-encoded in the OTLP/HTTP JSON the OpenTelemetry JS
// exporter emits (16- and 8-byte identifiers), not the base64 the generic proto3
// JSON mapping would use for bytes.
const traceIdHex = z.string().regex(/^[0-9a-f]{32}$/iu);
const spanIdHex = z.string().regex(/^[0-9a-f]{16}$/iu);
// proto3 JSON scalar encodings, enforced so a wrong-typed value is rejected
// rather than relayed. A uint64/fixed64 nanosecond timestamp is a decimal string
// (values exceed the safe integer range) or a JSON number when small; a uint32
// (dropped counts, fixed32 flags) is a bounded non-negative integer; a signed
// int64 (AnyValue intValue) is a decimal string or an integer number; a double
// is a JSON number or one of the proto3 special-value strings; bytes are base64.
// The 64-bit domains are also range-checked: decimal syntax alone would relay a
// string one past the fixed-width maximum, which the collector rejects.
const UINT64_MAX = 18_446_744_073_709_551_615n;
const INT64_MIN = -9_223_372_036_854_775_808n;
const INT64_MAX = 9_223_372_036_854_775_807n;
const withinBigIntRange =
  (min: bigint, max: bigint) =>
  (text: string): boolean => {
    try {
      const value = BigInt(text);
      return value >= min && value <= max;
    } catch {
      return false;
    }
  };
const uint32 = z.number().int().min(0).max(4_294_967_295);
// A uint64 nanosecond timestamp: a decimal string bounded by BigInt to the
// fixed-width maximum, or a JSON number confined to the safe-integer range so a
// larger value that has already lost precision is rejected rather than relayed.
const unixNano = z.union([
  z.string().regex(/^\d+$/u).refine(withinBigIntRange(0n, UINT64_MAX), {
    message: "unixNano string is outside the uint64 range",
  }),
  z.number().int().min(0).max(Number.MAX_SAFE_INTEGER),
]);
// A signed int64: a decimal string bounded by BigInt to the int64 range, or a
// JSON number confined to the safe-integer range for the same reason.
const int64Json = z.union([
  z
    .string()
    .regex(/^-?\d+$/u)
    .refine(withinBigIntRange(INT64_MIN, INT64_MAX), {
      message: "intValue string is outside the int64 range",
    }),
  z.number().int().min(Number.MIN_SAFE_INTEGER).max(Number.MAX_SAFE_INTEGER),
]);
const doubleJson = z.union([
  z.number(),
  z.enum(["NaN", "Infinity", "-Infinity"]),
]);
const base64 = z
  .string()
  .regex(/^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/u);

// AnyValue is a proto3 oneof: at most one value field may be set, and each is
// carried in its proto3 JSON encoding. It is recursive -- an array or kvlist
// value nests further values -- so the value and key-value schemas reference
// each other through z.lazy.
const anyValueSchema: z.ZodType = z.lazy(() =>
  z
    .object({
      stringValue: z.string().optional(),
      boolValue: z.boolean().optional(),
      intValue: int64Json.optional(),
      doubleValue: doubleJson.optional(),
      bytesValue: base64.optional(),
      arrayValue: z
        .object({ values: z.array(anyValueSchema).max(maxAttributes) })
        .optional(),
      kvlistValue: z
        .object({ values: z.array(keyValueSchema).max(maxAttributes) })
        .optional(),
    })
    .refine(
      (value) =>
        Object.values(value as Record<string, unknown>).filter(
          (field) => field !== undefined,
        ).length <= 1,
      { message: "AnyValue must set at most one value field" },
    ),
);
const keyValueSchema: z.ZodType = z.lazy(() =>
  z.object({ key: z.string(), value: anyValueSchema.optional() }),
);
const attributesSchema = z.array(keyValueSchema).max(maxAttributes).optional();

const spanSchema = z.object({
  traceId: traceIdHex,
  spanId: spanIdHex,
  traceState: z.string().optional(),
  parentSpanId: spanIdHex.optional(),
  flags: uint32.optional(),
  name: z.string(),
  // SpanKind is a proto enum, 0 (unspecified) through 5 (consumer).
  kind: z.number().int().min(0).max(5).optional(),
  startTimeUnixNano: unixNano.optional(),
  endTimeUnixNano: unixNano.optional(),
  attributes: attributesSchema,
  droppedAttributesCount: uint32.optional(),
  events: z
    .array(
      z.object({
        timeUnixNano: unixNano.optional(),
        name: z.string().optional(),
        attributes: attributesSchema,
        droppedAttributesCount: uint32.optional(),
      }),
    )
    .max(maxEvents)
    .optional(),
  // uint32 counters the OpenTelemetry JS exporter emits alongside truncated
  // event and link collections. Validated so a wrong-typed or negative count is
  // rejected rather than silently ignored and relayed to the collector.
  droppedEventsCount: uint32.optional(),
  links: z
    .array(
      z.object({
        traceId: traceIdHex.optional(),
        spanId: spanIdHex.optional(),
        traceState: z.string().optional(),
        attributes: attributesSchema,
        droppedAttributesCount: uint32.optional(),
        flags: uint32.optional(),
      }),
    )
    .max(maxLinks)
    .optional(),
  droppedLinksCount: uint32.optional(),
  status: z
    .object({
      message: z.string().optional(),
      // Status code is a proto enum, 0 (unset) through 2 (error).
      code: z.number().int().min(0).max(2).optional(),
    })
    .optional(),
});

const otlpTracePayloadSchema = z.object({
  resourceSpans: z
    .array(
      z.object({
        resource: z
          .object({
            attributes: attributesSchema,
            droppedAttributesCount: uint32.optional(),
          })
          .optional(),
        scopeSpans: z
          .array(
            z.object({
              scope: z
                .object({
                  name: z.string().optional(),
                  version: z.string().optional(),
                  attributes: attributesSchema,
                  droppedAttributesCount: uint32.optional(),
                })
                .optional(),
              spans: z.array(spanSchema).max(maxSpans).optional(),
              schemaUrl: z.string().optional(),
            }),
          )
          .max(maxScopeSpans)
          .optional(),
        schemaUrl: z.string().optional(),
      }),
    )
    .max(maxResourceSpans),
});

/**
 * Validates the OTLP/HTTP trace envelope against the supported subset of the
 * OTLP JSON schema -- not merely the three container arrays, but every nested
 * message and scalar in its proto3 JSON encoding: span ids are hex, timestamps
 * are nanosecond encodings, counts and flags are bounded uint32s, an AnyValue is
 * a oneof of typed values, and status, events, and links match their proto
 * shapes, all under fixed bounds. A bounded structural pre-pass caps nesting
 * depth so the recursive schema cannot overflow the stack, and any validator
 * exception is converted to a rejection. Unknown forward-compatible keys are
 * ignored, but a wrong-typed or malformed known field is rejected before the
 * payload reaches the collector, rather than accepted and relayed only for the
 * collector to reject it after the browser was told the export succeeded
 * (WEB-TRACE-02).
 */
function isOtlpTracePayload(payload: unknown): boolean {
  try {
    return (
      withinStructuralBudget(payload) &&
      otlpTracePayloadSchema.safeParse(payload).success
    );
  } catch {
    // A validator exception (for example a call-stack overflow on a shape the
    // budget somehow admitted) is a rejection, never a relay.
    return false;
  }
}

const exportBackstopTimeoutMs = 15_000;

/** Bounded, mutable delivery-health tally folded into the port snapshot. */
interface MutableDeliveryHealth {
  relayFailures: number;
  spanExportFailures: number;
  lastErrorType?: string;
}

/**
 * Builds a self-observation {@link MeterProvider} that folds the batch
 * processor's own span-processing counter into the delivery-health tally. The
 * processor emits one counter, `otel.sdk.processor.span.processed`, tagged with
 * an `error.type` attribute on every loss: `queue_full` when a span is dropped
 * because the buffer is full, and the exporter error name when a batch export
 * fails. Successful processing carries no `error.type` and is ignored. This is
 * the single accounting site for every span loss the SDK observes, rather than
 * only the export failures an out-of-band wrapper could see.
 */
function deliveryHealthMeterProvider(
  health: MutableDeliveryHealth,
): MeterProvider {
  const noop = createNoopMeter();
  const createReportingCounter = (
    name: string,
    options?: MetricOptions,
  ): Counter => {
    const inner = noop.createCounter(name, options);
    return {
      add(value: number, attributes?: Attributes): void {
        const errorType = attributes?.[ATTR_ERROR_TYPE];
        if (typeof errorType === "string") {
          // The processor reports the measurement as the number of spans lost in
          // this batch, so the tally advances by that count rather than by one:
          // a failed multi-span batch must not be undercounted as a single span.
          health.spanExportFailures += value;
          health.lastErrorType = errorType;
        }
        inner.add(value, attributes);
      },
    };
  };
  // Delegate every instrument to the no-op meter except the counter, whose `add`
  // is intercepted above. The no-op meter is a shared singleton, so a fresh
  // delegating meter is returned rather than mutating it.
  const meter: Meter = {
    createCounter: createReportingCounter,
    createGauge: (name, options) => noop.createGauge(name, options),
    createHistogram: (name, options) => noop.createHistogram(name, options),
    createObservableCounter: (name, options) =>
      noop.createObservableCounter(name, options),
    createObservableGauge: (name, options) =>
      noop.createObservableGauge(name, options),
    createObservableUpDownCounter: (name, options) =>
      noop.createObservableUpDownCounter(name, options),
    createUpDownCounter: (name, options) =>
      noop.createUpDownCounter(name, options),
    addBatchObservableCallback: (callback, observables) => {
      noop.addBatchObservableCallback(callback, observables);
    },
    removeBatchObservableCallback: (callback, observables) => {
      noop.removeBatchObservableCallback(callback, observables);
    },
  };
  return { getMeter: () => meter };
}

/**
 * Wraps a span exporter so a batch always receives a terminal result even when
 * the inner exporter never calls back. A wedged exporter would otherwise let the
 * batch processor's export timeout fire, which rejects the flush without
 * accounting the loss through the self-observation meter. Converting the stall
 * into a FAILED result routes it back through the processor's finish path (and
 * so the meter) exactly once; genuine results pass straight through. Accounting
 * lives in {@link deliveryHealthMeterProvider}, so this wrapper never records --
 * it only guarantees the callback the meter depends on.
 */
function backstopExporter(
  inner: SpanExporter,
  timeoutMs: number,
): SpanExporter {
  return {
    export(spans, resultCallback) {
      let settled = false;
      const settle = (result: ExportResult): void => {
        if (settled) {
          return;
        }
        settled = true;
        clearTimeout(timer);
        resultCallback(result);
      };
      const timer = setTimeout(() => {
        const error = new Error(
          "span export timed out before the exporter responded",
        );
        error.name = "SpanExportTimeout";
        settle({ code: ExportResultCode.FAILED, error });
      }, timeoutMs);
      // A synchronous throw from the inner exporter would otherwise bypass the
      // callback entirely, so the loss would go unaccounted until the timer
      // fired (or never, at shutdown). Normalize the throw into a FAILED result
      // and settle it now, routing it through the processor's finish path (and
      // so the self-observation meter) exactly once.
      try {
        inner.export(spans, settle);
      } catch (thrown) {
        settle({
          code: ExportResultCode.FAILED,
          error: thrown instanceof Error ? thrown : new Error(String(thrown)),
        });
      }
    },
    forceFlush: () => inner.forceFlush?.() ?? Promise.resolve(),
    shutdown: () => inner.shutdown(),
  };
}

/**
 * The batch processor config extended with the self-observation meter provider.
 * The bundled `sdk-trace-base` shim omits `selfObsMeterProvider` from its
 * constructor config type but forwards it to the underlying processor, so this
 * intersection re-adds the field for a typed hand-off.
 */
type SelfObservableBatchConfig = BufferConfig & {
  selfObsMeterProvider?: MeterProvider;
};

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

  const health: MutableDeliveryHealth = {
    relayFailures: 0,
    spanExportFailures: 0,
  };
  // Wrap the exporter so a wedged collector still yields a terminal result, and
  // wire the self-observation meter that folds every processor-accounted loss
  // (queue overflow and export failure) into the delivery-health tally.
  const exporter = backstopExporter(
    new OTLPTraceExporter({ url: tracing.tracesEndpoint }),
    exportBackstopTimeoutMs,
  );
  const processorConfig: SelfObservableBatchConfig = {
    selfObsMeterProvider: deliveryHealthMeterProvider(health),
  };
  const provider = new BasicTracerProvider({
    resource: resourceFromAttributes({
      [ATTR_SERVICE_NAME]: tracing.serviceName,
    }),
    sampler: new ParentBasedSampler({
      root: new TraceIdRatioBasedSampler(tracing.sampleRatio),
    }),
    spanProcessors: [new BatchSpanProcessor(exporter, processorConfig)],
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
      // A transient 408/429 or any 5xx means the collector could not accept the
      // relay: best-effort for the browser, but a delivery failure worth
      // surfacing in the bounded health diagnostic.
      health.relayFailures += 1;
      health.lastErrorType = "collector_unavailable";
      return "unavailable";
    } catch {
      // Best-effort: an unreachable collector never fails the browser request,
      // but the loss is still counted so the health diagnostic reflects it.
      health.relayFailures += 1;
      health.lastErrorType = "collector_unreachable";
      return "unavailable";
    }
  }

  const deliveryHealth = (): BffDeliveryHealthSnapshot =>
    health.lastErrorType === undefined
      ? {
          relayFailures: health.relayFailures,
          spanExportFailures: health.spanExportFailures,
        }
      : {
          lastErrorType: health.lastErrorType,
          relayFailures: health.relayFailures,
          spanExportFailures: health.spanExportFailures,
        };

  return {
    deliveryHealth,
    enabled: true,
    ingestTraces,
    shutdown: () => provider.shutdown(),
    startProxySpan,
  };
}
