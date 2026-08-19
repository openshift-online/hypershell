import type { GatewayProbe } from "@openshift-online/hypershell-gateway-management-ui";
import { describe, expect, it, vi } from "vitest";

import { createGatewayObservability } from "./gateway-observability";

const testProbe: GatewayProbe = {
  context: { correlationId: "correlation-1" },
  fields: { action: "list", failureKind: null, outcome: "started" },
  name: "gateway.workflow.started",
  occurredAt: "2026-08-06T18:00:00.000Z",
  schemaVersion: 1,
};

describe("gateway observability adapter", () => {
  it("fans out immutable probes and isolates a failing sink", () => {
    const clearMarks = vi.fn();
    const mark = vi.fn<
      (name: string, options?: PerformanceMarkOptions) => PerformanceMark
    >(() => ({}) as PerformanceMark);
    const observability = createGatewayObservability({
      additionalSinks: [
        {
          id: "failing-sink",
          publish() {
            throw new Error("unavailable");
          },
        },
      ],
      performanceTarget: { clearMarks, mark },
    });

    expect(() => {
      observability.probes.publish(testProbe);
    }).not.toThrow();

    expect(observability.recentProbes()).toEqual([testProbe]);
    expect(clearMarks).toHaveBeenCalledWith("gateway.workflow.started");
    expect(mark).toHaveBeenCalledWith("gateway.workflow.started", {
      detail: testProbe,
    });
    expect(observability.deliveryHealth()).toMatchObject({
      deliveryFailureCount: 1,
      diagnosticFailureCount: 0,
      lastFailure: { sinkId: "failing-sink" },
    });
    expect(observability.recentDeliveryFailures()).toEqual([
      expect.objectContaining({ sinkId: "failing-sink" }),
    ]);
  });

  it("records an out-of-band delivery failure into delivery health", () => {
    const observability = createGatewayObservability({
      performanceTarget: { clearMarks: vi.fn(), mark: vi.fn() },
    });

    observability.reportDeliveryFailure({
      errorType: "SpanExportError",
      probeName: "gateway.trace.export",
      schemaVersion: 0,
      sinkId: "gateway-trace",
    });

    expect(observability.deliveryHealth()).toMatchObject({
      deliveryFailureCount: 1,
      lastFailure: { sinkId: "gateway-trace" },
    });
    expect(observability.recentDeliveryFailures()).toEqual([
      expect.objectContaining({ probeName: "gateway.trace.export" }),
    ]);
  });

  it("provides deterministic workflow context through injected capabilities", () => {
    const observability = createGatewayObservability({
      createCorrelationId: () => "correlation-1",
      createTraceId: () => "0af7651916cd43dd8448eb211c80319c",
      now: () => "2026-08-06T18:00:00.000Z",
      performanceTarget: { clearMarks: vi.fn(), mark: vi.fn() },
    });

    expect(observability.runtime.createCorrelationId()).toBe("correlation-1");
    expect(observability.runtime.createTraceId()).toBe(
      "0af7651916cd43dd8448eb211c80319c",
    );
    expect(observability.runtime.now()).toBe("2026-08-06T18:00:00.000Z");
  });

  it("generates a valid W3C trace identifier by default", () => {
    const observability = createGatewayObservability({
      performanceTarget: { clearMarks: vi.fn(), mark: vi.fn() },
    });

    const traceId = observability.runtime.createTraceId();

    expect(traceId).toMatch(/^[0-9a-f]{32}$/);
    expect(traceId).not.toBe("0".repeat(32));
    expect(observability.runtime.createTraceId()).not.toBe(traceId);
  });
});
