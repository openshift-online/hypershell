import { fireEvent, render, screen } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { describe, expect, it } from "vitest";

import type { AgentSdlcData } from "../../../domain/agent-sdlc";
import { identiName } from "../../../domain/map/identiname";
import { parseRoute } from "../../../domain/navigation/route";
import { createMemoryHistory } from "../../routing/history-port";
import { RouterProvider } from "../../routing/router";
import { AgentSdlcView, type AgentSdlcViewProps } from "./agent-sdlc-view";

const EMPTY: AgentSdlcData = {
  phases: [],
  cards: [],
  lane: [],
  matrix: { hubClusters: [], rows: [] },
};

function renderView(overrides: Partial<AgentSdlcViewProps>): {
  history: ReturnType<typeof createMemoryHistory>;
} {
  const history = createMemoryHistory({ path: "/sdlc", search: "" });
  render(
    <IntlProvider locale="en" defaultLocale="en">
      <RouterProvider history={history}>
        <AgentSdlcView data={EMPTY} selectedBundleSeed={null} {...overrides} />
      </RouterProvider>
    </IntlProvider>,
  );
  return { history };
}

describe("AgentSdlcView board", () => {
  it("renders a column per board phase with its card count, and the card's rework meter", () => {
    const data: AgentSdlcData = {
      ...EMPTY,
      phases: [
        { key: "spec", sleSeconds: 100 },
        { key: "shipped", sleSeconds: 0 },
      ],
      cards: [
        {
          number: 10,
          title: "Add tab",
          url: "u",
          phase: "spec",
          timeInPhaseSeconds: 50,
          scope: null,
          escalation: null,
          rework: { count: 2, threshold: 3 },
          history: [],
        },
      ],
    };
    renderView({ data });

    // The Spec column header renders (Shipped is the matrix, not a board column).
    expect(screen.getByText("Spec")).toBeDefined();
    // The rework meter exposes its count/threshold as an accessible meter.
    const meter = screen.getByRole("meter", { name: "2 of 3 rejections" });
    expect(meter.getAttribute("aria-valuenow")).toBe("2");
    expect(meter.getAttribute("aria-valuemax")).toBe("3");
    // At 2 of 3 the remaining-until-block countdown shows (non-color cue).
    expect(
      screen.getByText(/1 more rejection blocks this item\./),
    ).toBeDefined();
  });

  it("shows time-in-phase to minute resolution for a young card", () => {
    const data: AgentSdlcData = {
      ...EMPTY,
      phases: [{ key: "spec", sleSeconds: 100_000 }],
      cards: [
        {
          number: 11,
          title: "eleven minutes in",
          url: "u",
          phase: "spec",
          timeInPhaseSeconds: 11 * 60,
          scope: null,
          escalation: null,
          rework: null,
          history: [],
        },
      ],
    };
    // Freeze the clock so the live-aging delta is zero and the snapshot age renders.
    renderView({ data, nowMs: Date.UTC(2026, 9, 15, 12, 0, 0) });

    // A card 11 minutes old reads "11m", not a misleading "0d 0h".
    expect(screen.getByText("11m")).toBeDefined();
  });
});

describe("AgentSdlcView escalations notification drawer", () => {
  it("shows the cleared state when the drawer is opened with an empty lane", () => {
    renderView({ data: EMPTY });
    // The escalations live behind the bell now; open the notification drawer.
    fireEvent.click(screen.getByRole("button", { name: "Escalations" }));
    expect(
      screen.getByText("Escalation lane clear - nothing blocked or stalled."),
    ).toBeDefined();
  });

  it("groups escalations by bucket when the drawer is opened", () => {
    const data: AgentSdlcData = {
      ...EMPTY,
      lane: [
        {
          number: 1,
          title: "blocked one",
          url: "u",
          phase: "code",
          timeInPhaseSeconds: 10,
          scope: null,
          escalation: "blocked",
          rework: null,
          history: [],
        },
        {
          number: 2,
          title: "stale one",
          url: "u",
          phase: "spec",
          timeInPhaseSeconds: 999,
          scope: null,
          escalation: "past-sle",
          rework: null,
          history: [],
        },
      ],
    };
    renderView({ data });
    fireEvent.click(screen.getByRole("button", { name: "Escalations" }));
    expect(screen.getByText("Blocked")).toBeDefined();
    expect(screen.getByText("Past SLE")).toBeDefined();
  });
});

describe("AgentSdlcView matrix", () => {
  it("shows the Unreleased row and cross-links a bundle to the map", () => {
    const data: AgentSdlcData = {
      ...EMPTY,
      matrix: {
        hubClusters: [
          {
            key: "env-a",
            cluster: "cluster-a",
            env: "prod",
            currentVersion: "v2",
            currentDigest: "d2",
          },
        ],
        rows: [
          {
            version: null,
            digest: null,
            date: null,
            unreleased: true,
            hubClusters: [],
            issues: [],
          },
          {
            version: "v2",
            digest: "d2",
            date: null,
            unreleased: false,
            hubClusters: ["env-a"],
            issues: [],
          },
        ],
      },
    };
    const { history } = renderView({ data });

    expect(screen.getByText("Unreleased")).toBeDefined();
    // The current-bundle marker reads with a non-color cue ("current" text).
    expect(screen.getAllByText("current").length).toBeGreaterThan(0);

    // Activating the bundle opens it on the map (shared freight bar + tab jump)
    // as a single addressable navigation (GLASS-NAV-08). The bundle reads by its
    // identiname (the stable two-word alias), not the raw version.
    fireEvent.click(screen.getByRole("link", { name: identiName("d2") }));
    const route = parseRoute(history.read());
    expect(route.tab).toBe("map");
    expect(route.bundle).toBe("d2");
  });

  it("renders a localized 'merged Nd ago' timestamp using the injected clock", () => {
    const nowMs = Date.UTC(2026, 9, 15, 12, 0, 0);
    const mergedAt = new Date(Date.UTC(2026, 9, 12, 12, 0, 0)).toISOString(); // 3 days earlier
    const data: AgentSdlcData = {
      ...EMPTY,
      matrix: {
        hubClusters: [],
        rows: [
          {
            version: "v9",
            digest: "d9",
            date: null,
            unreleased: false,
            hubClusters: [],
            issues: [
              {
                number: 700,
                title: "shipped thing",
                url: "u",
                mergedAt,
                mergedBy: "octo-dev",
              },
            ],
          },
        ],
      },
    };
    renderView({ data, nowMs });

    // Expand the single bundle row to reveal its issues.
    fireEvent.click(screen.getByRole("button", { name: "Details" }));
    expect(screen.getByText("merged 3 days ago by octo-dev")).toBeDefined();
  });
});
