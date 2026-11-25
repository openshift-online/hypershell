import { describe, expect, it, vi } from "vitest";

import {
  freezeTerminalProbe,
  type TerminalDrawerOpenedProbe,
} from "../../application/terminal/terminal-probes";
import { createTerminalObservability } from "./terminal-observability";

const probe = freezeTerminalProbe<TerminalDrawerOpenedProbe>(
  "terminal.drawer.opened",
  { correlationId: "c-1" },
  "2026-11-24T12:00:00.000Z",
  { signedIn: true },
);

describe("terminal observability", () => {
  // Verifies: WEB-TERM-13
  it("fans each fact out to the recorder and performance sinks", () => {
    const mark = vi.fn();
    const clearMarks = vi.fn();
    const observability = createTerminalObservability({
      performanceTarget: { clearMarks, mark },
    });

    observability.probes.publish(probe);

    expect(observability.recentProbes()).toEqual([probe]);
    expect(mark).toHaveBeenCalledWith("terminal.drawer.opened", {
      detail: probe,
    });
    expect(observability.recentDeliveryFailures()).toEqual([]);
  });

  // Verifies: WEB-TERM-13
  it("keeps delivering when one sink fails and records the failure", () => {
    const observability = createTerminalObservability({
      performanceTarget: {
        clearMarks: () => undefined,
        mark: () => {
          throw new Error("performance unavailable");
        },
      },
    });

    observability.probes.publish(probe);

    expect(observability.recentProbes()).toEqual([probe]);
    expect(observability.recentDeliveryFailures()).toHaveLength(1);
  });
});
