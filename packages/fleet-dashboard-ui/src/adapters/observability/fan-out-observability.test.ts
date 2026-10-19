import { describe, expect, it, vi } from "vitest";

import {
  e2eStreamFact,
  e2eWorkflow,
  sdlcWorkflow,
  type ObservabilityPort,
} from "../../application/observability";
import { createFanOutObservability } from "./fan-out-observability";

/** A port with every method stubbed to a no-op; override just what a test needs. */
function stubPort(
  overrides: Partial<ObservabilityPort> = {},
): ObservabilityPort {
  return {
    publishSdlcWorkflow: () => {
      /* noop */
    },
    publishE2eWorkflow: () => {
      /* noop */
    },
    publishE2eStream: () => {
      /* noop */
    },
    ...overrides,
  };
}

describe("createFanOutObservability", () => {
  it("delivers each probe to every sink in order", () => {
    const calls: string[] = [];
    const sink = (name: string): ObservabilityPort =>
      stubPort({
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

  it("fans out the e2e workflow and stream facts to every sink", () => {
    const calls: string[] = [];
    const sink = (name: string): ObservabilityPort =>
      stubPort({
        publishE2eWorkflow: (probe) => {
          calls.push(`${name}:${probe.name}:${probe.outcome}`);
        },
        publishE2eStream: (fact) => {
          calls.push(`${name}:${fact.kind}:${String(fact.droppedEntries)}`);
        },
      });
    const fanOut = createFanOutObservability([sink("a"), sink("b")]);

    fanOut.publishE2eWorkflow(e2eWorkflow("e2e.runs.load", "started"));
    fanOut.publishE2eStream(e2eStreamFact("dropped-entry", 3));

    expect(calls).toEqual([
      "a:e2e.runs.load:started",
      "b:e2e.runs.load:started",
      "a:dropped-entry:3",
      "b:dropped-entry:3",
    ]);
  });

  it("is a no-op with no sinks", () => {
    const fanOut = createFanOutObservability([]);
    expect(() => {
      fanOut.publishSdlcWorkflow(sdlcWorkflow("failed"));
      fanOut.publishE2eWorkflow(e2eWorkflow("e2e.run.load", "failed"));
      fanOut.publishE2eStream(e2eStreamFact("connect"));
    }).not.toThrow();
  });

  it("isolates a throwing sink: the others still run and the caller is unaffected", () => {
    const after: string[] = [];
    const throwing = stubPort({
      publishSdlcWorkflow: () => {
        throw new Error("sink boom");
      },
      publishE2eStream: () => {
        throw new Error("sink boom");
      },
    });
    const healthy = stubPort({
      publishSdlcWorkflow: (probe) => {
        after.push(probe.outcome);
      },
      publishE2eStream: (fact) => {
        after.push(fact.kind);
      },
    });
    const fanOut = createFanOutObservability([throwing, healthy]);

    expect(() => {
      fanOut.publishSdlcWorkflow(sdlcWorkflow("succeeded"));
      fanOut.publishE2eStream(e2eStreamFact("reconnect"));
    }).not.toThrow();
    expect(after).toEqual(["succeeded", "reconnect"]);
  });

  it("passes the typed probe through unchanged", () => {
    const sink = stubPort({ publishSdlcWorkflow: vi.fn() });
    const fanOut = createFanOutObservability([sink]);
    fanOut.publishSdlcWorkflow(sdlcWorkflow("cancelled"));
    /* eslint-disable-next-line @typescript-eslint/unbound-method -- asserting on the vi.fn mock */
    expect(sink.publishSdlcWorkflow).toHaveBeenCalledWith({
      name: "sdlc.load",
      outcome: "cancelled",
    });
  });
});
