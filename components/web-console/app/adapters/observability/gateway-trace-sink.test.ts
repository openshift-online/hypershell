import type {
  GatewayAction,
  GatewayProbe,
} from "@openshift-online/hypershell-gateway-management-ui";
import { SpanStatusCode } from "@opentelemetry/api";
import {
  AlwaysOnSampler,
  BasicTracerProvider,
  InMemorySpanExporter,
  ParentBasedSampler,
  SimpleSpanProcessor,
  type ReadableSpan,
} from "@opentelemetry/sdk-trace-base";
import { beforeEach, describe, expect, it } from "vitest";

import { createGatewayTraceSink } from "./gateway-trace-sink";

const traceId = "0af7651916cd43dd8448eb211c80319c";
const parentSpanId = "aaaaaaaaaaaaaaaa";
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

function testTracer() {
  const exporter = new InMemorySpanExporter();
  const provider = new BasicTracerProvider({
    sampler: new ParentBasedSampler({ root: new AlwaysOnSampler() }),
    spanProcessors: [new SimpleSpanProcessor(exporter)],
  });
  return { exporter, tracer: provider.getTracer("test") };
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
      generateSpanId: () => parentSpanId,
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
    // The workflow span descends from the manufactured remote parent, so the
    // trace joins the id the caller propagates end to end.
    expect(workflow.parentSpanContext?.spanId).toBe(parentSpanId);
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
    const harness = testTracer();
    const unsampled = createGatewayTraceSink(harness.tracer, {
      generateSpanId: () => parentSpanId,
      isSampled: () => false,
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
});
