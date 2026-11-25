import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { describe, expect, it } from "vitest";

import type { AgentSdlcData } from "../../../domain/agent-sdlc";
import type { SdlcAgentsData } from "../../../domain/sdlc-agents";
import type { FleetData } from "../../../domain/fleet";
import type { Plane } from "../../../domain/plane";
import type { PromotionData } from "../../../domain/promotion";
import type {
  FleetApi,
  InstancesData,
  TopologyData,
} from "../../../application/ports";
import { FleetApiProvider } from "../../query/api-context";
import { createMemoryHistory } from "../../routing/history-port";
import { RouterProvider } from "../../routing/router";
import { App } from "../app";
import { EMPTY_NOW } from "../../../domain/delivery";

function plane<T>(data: T): Plane<T> {
  return {
    data,
    generatedAt: "2026-01-01T00:00:00Z",
    stale: false,
    error: null,
  };
}

// Server-supplied delivery/Argo fields every PromotionEnvironment carries; these
// tests only exercise selection, so they default to absent.
const envExtras = {
  activeDigest: null,
  proposedDigest: null,
  role: null,
  provider: null,
  envLabel: null,
  cluster: null,
  argoHealth: null,
  argoSync: null,
  syncRetries: null,
  consoleUrl: null,
  grafanaUrl: null,
  argoUrl: null,
  prState: null,
  prUrl: null,
  analysisUrl: null,
  upToDate: false,
} as const;

function renderApp(
  overrides: { promotion?: PromotionData; topology?: TopologyData } = {},
): void {
  const defaultPromotion: PromotionData = {
    order: ["staging"],
    environments: {
      staging: {
        name: "staging",
        activeRelease: "v2025",
        proposedRelease: null,
        gates: [],
        ...envExtras,
      },
    },
    releases: ["v2025"],
    releaseByDigest: {},
    frontier: null,
  };
  const promotion = overrides.promotion ?? defaultPromotion;
  const topology = overrides.topology ?? {};
  const api = {
    getFleet: () => Promise.resolve(plane<FleetData>({ instances: [] })),
    getPromotion: () => Promise.resolve(plane(promotion)),
    getTopology: () => Promise.resolve(plane<TopologyData>(topology)),
    getInstances: () =>
      Promise.resolve(plane<InstancesData>({ instances: [] })),
    getSdlc: () =>
      Promise.resolve(
        plane<AgentSdlcData>({
          phases: [],
          cards: [],
          lane: [],
          matrix: { hubClusters: [], rows: [] },
        }),
      ),
    getDelivery: () =>
      Promise.resolve(
        plane({
          enabled: false,
          stages: [],
          changes: [],
          train: [],
          now: EMPTY_NOW,
          releaseDiagnostics: null,
          omitted: 0,
          sources: {},
        }),
      ),
    getE2eRuns: () =>
      Promise.resolve(
        plane({
          enabled: true,
          runs: [],
          truncated: false,
          windowSeconds: 0,
          limit: 0,
        }),
      ),
    getE2eRun: () =>
      Promise.resolve(
        plane({
          id: "",
          runId: "",
          tenant: "",
          cluster: "",
          bodyCluster: null,
          namespace: "",
          revision: "",
          sourceRevision: null,
          startedAt: "",
          endedAt: null,
          durationMs: null,
          verdict: "unknown" as const,
          reportingComplete: false,
          hasStructuredSteps: false,
          stepCount: 0,
          plannedSteps: [],
          steps: [],
          failure: null,
          logs: [],
          logsTruncated: false,
        }),
      ),
    getE2eDiagnostics: () => Promise.resolve(plane({ diagnostics: [] })),
    getE2eRunLogs: () =>
      Promise.resolve(plane({ entries: [], nextCursor: null, hasMore: false })),
    getSdlcAgents: () =>
      Promise.resolve(plane<SdlcAgentsData>({ envs: [], agents: [] })),
    streamE2eRun: () => () => {
      /* unsubscribe */
    },
  } satisfies FleetApi;

  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <IntlProvider locale="en" defaultLocale="en">
      <QueryClientProvider client={queryClient}>
        <FleetApiProvider value={api}>
          <RouterProvider history={createMemoryHistory()}>
            <App />
          </RouterProvider>
        </FleetApiProvider>
      </QueryClientProvider>
    </IntlProvider>,
  );
}

describe("TopologyMap details drawer", () => {
  it("closes on Escape when open, and leaves Escape inert when closed", async () => {
    renderApp();

    // Open the drawer by selecting a map node (the <g role="button"> labelled
    // with the node id).
    const [node] = await screen.findAllByRole("button", { name: "staging" });
    if (!node) {
      throw new Error("expected a selectable 'staging' map node");
    }
    fireEvent.click(node);

    // The drawer is open: its close button is present.
    expect(
      await screen.findByRole("button", { name: "Close details" }),
    ).toBeDefined();

    // Escape closes it.
    fireEvent.keyDown(document, { key: "Escape" });
    expect(screen.queryByRole("button", { name: "Close details" })).toBeNull();

    // With the drawer already closed, Escape is a no-op (no throw, still closed).
    fireEvent.keyDown(document, { key: "Escape" });
    expect(screen.queryByRole("button", { name: "Close details" })).toBeNull();
  });

  it("leaves the drawer open when Escape targets a nested dialog (FLT-NAV-15)", async () => {
    renderApp();

    const [node] = await screen.findAllByRole("button", { name: "staging" });
    if (!node) {
      throw new Error("expected a selectable 'staging' map node");
    }
    fireEvent.click(node);
    expect(
      await screen.findByRole("button", { name: "Close details" }),
    ).toBeDefined();

    // Escape raised from inside a nested dialog (the expanded log modal) must be
    // handled by that dialog, not collapse the whole gate drawer underneath it.
    const dialog = document.createElement("div");
    dialog.setAttribute("role", "dialog");
    const inside = document.createElement("button");
    dialog.appendChild(inside);
    document.body.appendChild(dialog);
    try {
      fireEvent.keyDown(inside, { key: "Escape" });
      expect(
        screen.queryByRole("button", { name: "Close details" }),
      ).not.toBeNull();
    } finally {
      document.body.removeChild(dialog);
    }
  });
});

describe("TopologyMap physical-cluster containers", () => {
  // Verifies: FLT-MAP-08.03
  it("labels the container with the cluster and drops the cards' own label", async () => {
    renderApp({
      promotion: {
        order: ["hub0"],
        environments: {
          hub0: {
            name: "hub0",
            activeRelease: "v1",
            proposedRelease: null,
            gates: [],
            ...envExtras,
            role: "hub",
            provider: "ibm",
            cluster: "phys-a",
          },
        },
        releases: ["v1"],
        releaseByDigest: {},
        frontier: null,
      },
      topology: {
        hub0: {
          instance: "hub0",
          hub: { instance: "hub0", dnsLabel: null, remoteSpokes: [] },
          spokes: [{ name: "hub0-co" }],
        },
      },
    });

    const group = await screen.findByRole("group", {
      name: "Physical cluster phys-a: hub0, hub0-co",
    });
    expect(group.textContent).toContain("phys-a");
    // Both nodes render as their own cards.
    expect(
      (await screen.findAllByRole("button", { name: "hub0-co" })).length,
    ).toBeGreaterThan(0);
    // The cluster name appears once on the map: on the container only.
    const svg = group.closest("svg");
    const labels = Array.from(svg?.querySelectorAll("text") ?? []).filter(
      (t) => t.textContent === "phys-a",
    );
    expect(labels).toHaveLength(1);
  });
});

describe("TopologyMap remote spokes without delivery data", () => {
  // Verifies: FLT-MAP-08.05
  it("draws the box without raising the health-unavailable alarm", async () => {
    renderApp({
      promotion: {
        order: ["hub0"],
        environments: {
          hub0: {
            name: "hub0",
            activeRelease: "v1",
            proposedRelease: null,
            gates: [],
            ...envExtras,
            role: "hub",
            provider: "ibm",
            argoHealth: "Healthy",
          },
        },
        releases: ["v1"],
        releaseByDigest: {},
        frontier: null,
      },
      topology: {
        hub0: {
          instance: "hub0",
          hub: { instance: "hub0", dnsLabel: null, remoteSpokes: ["hub0-far"] },
          spokes: [],
        },
      },
    });
    expect(
      (await screen.findAllByRole("button", { name: "hub0-far" })).length,
    ).toBeGreaterThan(0);
    expect(screen.queryByRole("status")).toBeNull();
  });
});
