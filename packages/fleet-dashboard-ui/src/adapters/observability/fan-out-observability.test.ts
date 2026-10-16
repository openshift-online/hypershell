import { describe, expect, it, vi } from "vitest";

import {
  sdlcWorkflow,
  type ObservabilityPort,
} from "../../application/observability";
import { createFanOutObservability } from "./fan-out-observability";

describe("createFanOutObservability", () => {
  it("delivers each probe to every sink in order", () => {
    const calls: string[] = [];
    const sink = (name: string): ObservabilityPort => ({
      publishSdlcWorkflow: (probe) => {
        calls.push(`${name}:${probe.outcome}`);
      },
    });
    const fanOut = createFanOutObservability([sink("a"), sink("b")]);

    fanOut.publishSdlcWorkflow(sdlcWorkflow("started"));
    fanOut.publishSdlcWorkflow(sdlcWorkflow("succeeded"));

    expect(calls).toEqual([
      "a:started",
      "b:started",
      "a:succeeded",
      "b:succeeded",
    ]);
  });

  it("is a no-op with no sinks", () => {
    const fanOut = createFanOutObservability([]);
    expect(() => {
      fanOut.publishSdlcWorkflow(sdlcWorkflow("failed"));
    }).not.toThrow();
  });

  it("isolates a throwing sink: the others still run and the caller is unaffected", () => {
    const after: string[] = [];
    const throwing: ObservabilityPort = {
      publishSdlcWorkflow: () => {
        throw new Error("sink boom");
      },
    };
    const healthy: ObservabilityPort = {
      publishSdlcWorkflow: (probe) => {
        after.push(probe.outcome);
      },
    };
    const fanOut = createFanOutObservability([throwing, healthy]);

    expect(() => {
      fanOut.publishSdlcWorkflow(sdlcWorkflow("succeeded"));
    }).not.toThrow();
    expect(after).toEqual(["succeeded"]);
  });

  it("passes the typed probe through unchanged", () => {
    const sink: ObservabilityPort = { publishSdlcWorkflow: vi.fn() };
    const fanOut = createFanOutObservability([sink]);
    fanOut.publishSdlcWorkflow(sdlcWorkflow("cancelled"));
    /* eslint-disable-next-line @typescript-eslint/unbound-method -- asserting on the vi.fn mock */
    expect(sink.publishSdlcWorkflow).toHaveBeenCalledWith({
      name: "sdlc.load",
      outcome: "cancelled",
    });
  });
});
