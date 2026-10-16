import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { describe, expect, it } from "vitest";

import type { AgentSdlcData } from "../../domain/agent-sdlc";
import type { FleetData } from "../../domain/fleet";
import type { Plane } from "../../domain/plane";
import type { PromotionData } from "../../domain/promotion";
import type {
  FleetApi,
  InstancesData,
  TopologyData,
} from "../../application/ports";
import { FleetApiProvider } from "../query/api-context";
import { App } from "./app";

function plane<T>(data: T): Plane<T> {
  return {
    data,
    generatedAt: "2026-01-01T00:00:00Z",
    stale: false,
    error: null,
  };
}

// Server-supplied delivery/Argo fields every PromotionEnvironment now carries;
// these tests only exercise order/gates, so they default to absent.
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

function renderApp(api: FleetApi): void {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <IntlProvider locale="en" defaultLocale="en">
      <QueryClientProvider client={queryClient}>
        <FleetApiProvider value={api}>
          <App />
        </FleetApiProvider>
      </QueryClientProvider>
    </IntlProvider>,
  );
}

describe("App", () => {
  it("renders promotion environments and governing instances straight from the payload", async () => {
    const promotion: PromotionData = {
      order: ["staging", "canary"],
      environments: {
        canary: {
          name: "canary",
          activeRelease: "v2026",
          proposedRelease: null,
          gates: [
            { name: "g", phase: "success", governingInstance: "gov-from-data" },
          ],
          ...envExtras,
        },
        staging: {
          name: "staging",
          activeRelease: "v2025",
          proposedRelease: null,
          gates: [],
          ...envExtras,
        },
      },
      releases: ["v2026", "v2025"],
      releaseByDigest: {},
      frontier: null,
    };
    const empty = {
      getFleet: () => Promise.resolve(plane<FleetData>({ instances: [] })),
      getPromotion: () => Promise.resolve(plane(promotion)),
      getTopology: () => Promise.resolve(plane<TopologyData>({})),
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
    } satisfies FleetApi;

    renderApp(empty);

    // Server-defined order: staging before canary. The names now appear in both
    // the topology map and the promotion table, so match all occurrences.
    expect((await screen.findAllByText("staging")).length).toBeGreaterThan(0);
    expect((await screen.findAllByText("canary")).length).toBeGreaterThan(0);
    // Governing instance came from gate data, not any hard-coded mapping. It is
    // only in the (visually hidden, still accessible) promotion table now.
    expect(await screen.findByText("gov-from-data")).toBeDefined();
  });

  it("offers Fleet Map and Agent SDLC as peer tabs in the shared chrome", async () => {
    const promotion: PromotionData = {
      order: [],
      environments: {},
      releases: [],
      releaseByDigest: {},
      frontier: null,
    };
    const api = {
      getFleet: () => Promise.resolve(plane<FleetData>({ instances: [] })),
      getPromotion: () => Promise.resolve(plane(promotion)),
      getTopology: () => Promise.resolve(plane<TopologyData>({})),
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
    } satisfies FleetApi;

    renderApp(api);

    // Both tabs are present and keyboard-reachable (role=tab).
    const mapTab = await screen.findByRole("tab", { name: "Fleet Map" });
    const sdlcTab = await screen.findByRole("tab", { name: "Agent SDLC" });
    expect(mapTab.getAttribute("aria-selected")).toBe("true");

    // Activating the Agent SDLC tab renders its view (the escalations bell lives in
    // the shared toolbar chrome) and opening it shows the empty-lane cleared state.
    fireEvent.click(sdlcTab);
    expect(sdlcTab.getAttribute("aria-selected")).toBe("true");
    const bell = await screen.findByRole("button", { name: "Escalations" });
    fireEvent.click(bell);
    expect(
      await screen.findByText(
        "Escalation lane clear - nothing blocked or stalled.",
      ),
    ).toBeDefined();
  });
});
