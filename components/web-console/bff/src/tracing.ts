/**
 * Application-owned tracing port. The Fastify app depends on this contract
 * only; the concrete OpenTelemetry adapter lives in
 * `adapters/observability/otel-tracing.ts`, and the bootstrap wires the two.
 * Keeping the port free of any tracing vendor lets route code stay inside the
 * telemetry import ban that `eslint.config.mjs` enforces.
 */

/** Bounded outcome class recorded on a proxy span; never a raw status code. */
export type ProxyOutcome =
  "client_error" | "server_error" | "success" | "timeout";

/** W3C trace context a proxied request carries to the upstream API. */
export interface UpstreamTraceContext {
  traceparent: string;
  tracestate?: string;
}

/** One in-flight BFF server span for a proxied request. */
export interface ProxySpan {
  /**
   * The valid upstream trace context that references this server span, or
   * `undefined` when tracing is disabled. The value never echoes a malformed
   * inbound header; it is derived from the span this BFF started.
   */
  upstream(): UpstreamTraceContext | undefined;
  end(outcome: ProxyOutcome, statusCode: number): void;
}

/** Result of relaying a browser OTLP payload to the collector. */
export type TelemetryIngestResult = "accepted" | "rejected" | "unavailable";

export interface StartProxySpanInput {
  correlationId: string;
  method: string;
  routeTemplate: string;
  traceparent?: string;
  tracestate?: string;
}

/** Application-owned port for BFF request tracing and browser telemetry relay. */
export interface BffTracing {
  readonly enabled: boolean;
  /**
   * Starts a server span for a proxied request. A valid inbound W3C context is
   * continued; an absent or malformed inbound context starts a new trace.
   */
  startProxySpan(input: StartProxySpanInput): ProxySpan;
  /**
   * Relays a browser OTLP/HTTP trace payload to the configured collector.
   * Rejects a payload that is not well-formed OTLP; a collector that is
   * unreachable is reported as unavailable rather than surfaced as an error.
   */
  ingestTraces(payload: unknown): Promise<TelemetryIngestResult>;
  /** Flushes buffered spans and shuts the exporter down on server shutdown. */
  shutdown(): Promise<void>;
}

const disabledProxySpan: ProxySpan = {
  end: () => undefined,
  upstream: () => undefined,
};

/** A tracing port with tracing turned off: no spans, no context, no relay. */
export const disabledTracing: BffTracing = {
  enabled: false,
  ingestTraces: () => Promise.resolve("unavailable"),
  shutdown: () => Promise.resolve(),
  startProxySpan: () => disabledProxySpan,
};
