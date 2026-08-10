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

  it("provides deterministic workflow context through injected capabilities", () => {
    const observability = createGatewayObservability({
      createCorrelationId: () => "correlation-1",
      now: () => "2026-08-06T18:00:00.000Z",
      performanceTarget: { clearMarks: vi.fn(), mark: vi.fn() },
    });

    expect(observability.runtime.createCorrelationId()).toBe("correlation-1");
    expect(observability.runtime.now()).toBe("2026-08-06T18:00:00.000Z");
  });
});
