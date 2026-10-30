import { describe, expect, it } from "vitest";

import { isReplaceAction, reduceRoute, type NavAction } from "./actions";
import { DEFAULT_ROUTE, GATE_LIVE_ATTEMPT, type RouteState } from "./route";

function apply(state: RouteState, ...actions: NavAction[]): RouteState {
  return actions.reduce(reduceRoute, state);
}

describe("reduceRoute", () => {
  // Verifies: FLT-NAV-08.01
  it("switches tabs and keeps each tab's own state (FLT-NAV-08)", () => {
    const state = apply(
      DEFAULT_ROUTE,
      { type: "tab", tab: "e2e" },
      { type: "e2eSelectRun", run: "t/n/r" },
      { type: "mapSelectNode", id: "n1" },
    );
    // The node selection landed on the map slice; switching tabs does not leak it.
    expect(state.e2e.run).toBe("t/n/r");
    expect(state.map.node).toBe("n1");
    const back = reduceRoute(state, { type: "tab", tab: "map" });
    expect(back.e2e.run).toBe("t/n/r");
    expect(back.map.node).toBe("n1");
  });

  it("toggles the shared bundle and clears a map node/gate on select", () => {
    const withNode = reduceRoute(DEFAULT_ROUTE, {
      type: "mapSelectNode",
      id: "n1",
    });
    const on = reduceRoute(withNode, { type: "toggleBundle", seed: "d1" });
    expect(on.bundle).toBe("d1");
    expect(on.map.node).toBeNull();
    const off = reduceRoute(on, { type: "toggleBundle", seed: "d1" });
    expect(off.bundle).toBeNull();
  });

  it("openHistory jumps to the E2E tab with the run selected (FLT-NAV-08)", () => {
    const state = reduceRoute(DEFAULT_ROUTE, {
      type: "openHistory",
      run: "t/n/r",
    });
    expect(state.tab).toBe("e2e");
    expect(state.e2e.run).toBe("t/n/r");
    expect(state.e2e.drawer).toBe(true);
  });

  // Verifies: FLT-NAV-11.01
  it("openHistory carries the gate step scope and Logs sub-tab into run history (FLT-NAV-11)", () => {
    const state = apply(
      DEFAULT_ROUTE,
      { type: "mapSelectGate", id: "g1" },
      // View step s1's logs inside the gate drawer.
      { type: "gateStep", step: "s1" },
      // "View history" for the same run and attempt.
      { type: "openHistory", run: "t/n/r" },
    );
    expect(state.e2e.run).toBe("t/n/r");
    // The step scope and Logs sub-tab survive the jump instead of resetting to
    // whole-run Progress.
    expect(state.e2e.exec.step).toBe("s1");
    expect(state.e2e.exec.tab).toBe("logs");
  });

  it("openHistory keeps an exec context already addressing the same run (FLT-NAV-11)", () => {
    const state = apply(
      DEFAULT_ROUTE,
      // The E2E tab already addresses run t/n/r with its own step scope.
      { type: "tab", tab: "e2e" },
      { type: "e2eSelectRun", run: "t/n/r" },
      { type: "e2eStep", step: "s2" },
      // A gate drawer scoped to a different step must not overwrite it.
      { type: "mapSelectGate", id: "g1" },
      { type: "gateStep", step: "s1" },
      { type: "openHistory", run: "t/n/r" },
    );
    expect(state.e2e.exec.step).toBe("s2");
  });

  it("openBundleOnMap sets the bundle and switches to the map (FLT-NAV-08)", () => {
    const state = apply(
      DEFAULT_ROUTE,
      { type: "tab", tab: "sdlc" },
      { type: "openBundleOnMap", seed: "sha256:xyz" },
    );
    expect(state.tab).toBe("map");
    expect(state.bundle).toBe("sha256:xyz");
  });

  it("toggles an expanded shipped matrix row by seed (FLT-NAV-13)", () => {
    const open = reduceRoute(DEFAULT_ROUTE, {
      type: "sdlcShippedRow",
      seed: "v9",
    });
    expect(open.sdlc.shippedRow).toBe("v9");
    const closed = reduceRoute(open, { type: "sdlcShippedRow", seed: "v9" });
    expect(closed.sdlc.shippedRow).toBeNull();
  });

  it("closes the whole SDLC panel", () => {
    const state = apply(
      DEFAULT_ROUTE,
      { type: "sdlcToggleEscalations" },
      { type: "sdlcOpenEscalationCard", card: 5 },
      { type: "sdlcClosePanel" },
    );
    expect(state.sdlc.escalations).toBe(false);
    expect(state.sdlc.card).toBeNull();
  });

  it("maps gate log-search and expanded-viewer through the exec contract", () => {
    const state = apply(
      DEFAULT_ROUTE,
      { type: "mapSelectGate", id: "g1" },
      { type: "gateLogQuery", value: "oops" },
      { type: "gateLogExpanded", value: true },
      { type: "gateStepSort", sort: { col: 1, dir: "asc" } },
    );
    expect(state.map.exec.logQuery).toBe("oops");
    expect(state.map.exec.logExpanded).toBe(true);
    expect(state.map.exec.stepSort).toEqual({ col: 1, dir: "asc" });
  });

  it("makes node and gate mutually exclusive", () => {
    const node = reduceRoute(DEFAULT_ROUTE, {
      type: "mapSelectNode",
      id: "n1",
    });
    const gate = reduceRoute(node, { type: "mapSelectGate", id: "g1" });
    expect(gate.map.node).toBeNull();
    expect(gate.map.gate).toBe("g1");
  });

  it("fully closes the map drawer including the shared bundle", () => {
    const state = apply(
      DEFAULT_ROUTE,
      { type: "toggleBundle", seed: "d1" },
      { type: "mapSelectNode", id: "n1" },
    );
    const closed = reduceRoute(state, { type: "mapClose" });
    expect(closed.map.node).toBeNull();
    expect(closed.bundle).toBeNull();
  });

  it("preserves the gate exec view across an attempt switch (FLT-NAV-05/10)", () => {
    // The auto-pin on open and a pasted deep link must keep their addressed sub-tab
    // and step; a missing step just resolves to whole-run logs in the new attempt.
    const state = apply(
      DEFAULT_ROUTE,
      { type: "mapSelectGate", id: "g1" },
      { type: "gatePin", attempt: "t/n/r1" },
      { type: "gateExecTab", tab: "logs" },
      { type: "gateStep", step: "s1" },
    );
    expect(state.map.exec.tab).toBe("logs");
    const repinned = reduceRoute(state, { type: "gatePin", attempt: "t/n/r2" });
    expect(repinned.map.attempt).toBe("t/n/r2");
    expect(repinned.map.exec.tab).toBe("logs");
    expect(repinned.map.exec.step).toBe("s1");
  });

  it("distinguishes the live/latest gate mode from a pinned attempt (FLT-NAV-10)", () => {
    const live = apply(
      DEFAULT_ROUTE,
      { type: "mapSelectGate", id: "g1" },
      { type: "gatePin", attempt: GATE_LIVE_ATTEMPT },
    );
    expect(live.map.attempt).toBe(GATE_LIVE_ATTEMPT);
    const pinned = reduceRoute(live, { type: "gatePin", attempt: "t/n/r1" });
    expect(pinned.map.attempt).toBe("t/n/r1");
  });

  it("selecting a step jumps to the Logs sub-tab", () => {
    const state = apply(
      DEFAULT_ROUTE,
      { type: "e2eSelectRun", run: "t/n/r" },
      { type: "e2eStep", step: "s1" },
    );
    expect(state.e2e.exec.tab).toBe("logs");
    expect(state.e2e.exec.step).toBe("s1");
    // Whole-run scope keeps the current tab.
    const whole = reduceRoute(state, { type: "e2eStep", step: null });
    expect(whole.e2e.exec.step).toBeNull();
    expect(whole.e2e.exec.tab).toBe("logs");
  });

  // Verifies: FLT-NAV-12.01
  it("opening then closing the run drawer preserves the selection (FLT-NAV-12)", () => {
    const state = reduceRoute(DEFAULT_ROUTE, {
      type: "e2eSelectRun",
      run: "t/n/r",
    });
    expect(state.e2e.drawer).toBe(true);
    const closed = reduceRoute(state, { type: "e2eDrawer", open: false });
    expect(closed.e2e.run).toBe("t/n/r");
    expect(closed.e2e.drawer).toBe(false);
  });

  it("changing the selected run resets the exec view (FLT-NAV-18)", () => {
    const state = apply(
      DEFAULT_ROUTE,
      { type: "e2eSelectRun", run: "t/n/r1" },
      { type: "e2eStep", step: "s1" },
    );
    expect(state.e2e.exec.step).toBe("s1");
    const other = reduceRoute(state, { type: "e2eSelectRun", run: "t/n/r2" });
    expect(other.e2e.exec.step).toBeNull();
  });

  it("opens an escalation card while keeping the list open (FLT-NAV-13)", () => {
    const state = apply(
      DEFAULT_ROUTE,
      { type: "tab", tab: "sdlc" },
      { type: "sdlcToggleEscalations" },
      { type: "sdlcOpenEscalationCard", card: 511 },
    );
    expect(state.sdlc.escalations).toBe(true);
    expect(state.sdlc.card).toBe(511);
    // Browser Back closing the card returns to the still-open list.
    const list = reduceRoute(state, { type: "sdlcCard", card: null });
    expect(list.sdlc.card).toBeNull();
    expect(list.sdlc.escalations).toBe(true);
  });

  it("a direct board-card selection closes the escalations drawer", () => {
    const state = apply(
      DEFAULT_ROUTE,
      { type: "sdlcToggleEscalations" },
      { type: "sdlcCard", card: 42 },
    );
    expect(state.sdlc.card).toBe(42);
    expect(state.sdlc.escalations).toBe(false);
  });

  it("selecting a shipped issue clears a board card and vice versa", () => {
    const card = reduceRoute(DEFAULT_ROUTE, { type: "sdlcCard", card: 1 });
    const shipped = reduceRoute(card, {
      type: "sdlcShipped",
      shipped: { issue: 2, version: "v1" },
    });
    expect(shipped.sdlc.card).toBeNull();
    expect(shipped.sdlc.shipped).toBe(2);
    expect(shipped.sdlc.shippedVersion).toBe("v1");
  });
});

describe("isReplaceAction (FLT-NAV-06)", () => {
  it("coalesces text input but pushes meaningful navigation", () => {
    expect(isReplaceAction({ type: "sdlcQuery", value: "a" })).toBe(true);
    expect(isReplaceAction({ type: "e2eQuery", value: "a" })).toBe(true);
    expect(isReplaceAction({ type: "gateLogQuery", value: "a" })).toBe(true);
    expect(isReplaceAction({ type: "e2eLogQuery", value: "a" })).toBe(true);
    expect(isReplaceAction({ type: "tab", tab: "e2e" })).toBe(false);
    expect(isReplaceAction({ type: "e2eSelectRun", run: "t/n/r" })).toBe(false);
    expect(
      isReplaceAction({ type: "e2eRunSort", sort: { col: 1, dir: "asc" } }),
    ).toBe(false);
  });
});
