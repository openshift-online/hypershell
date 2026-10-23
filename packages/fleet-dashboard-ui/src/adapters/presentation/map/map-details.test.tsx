import { render, screen } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { describe, expect, it } from "vitest";

import type { FleetData } from "../../../domain/fleet";
import type {
  PromotionData,
  PromotionEnvironment,
} from "../../../domain/promotion";
import type { SdlcAgentMetrics } from "../../../domain/sdlc-agents";
import { buildMapModel } from "../../../domain/map/model";
import { createMemoryHistory } from "../../routing/history-port";
import { RouterProvider } from "../../routing/router";
import { MapDetails } from "./map-details";

// Server-supplied fields every PromotionEnvironment carries; only syncRetries
// matters here, so the rest default to absent.
function env(
  overrides: Partial<PromotionEnvironment> & { name: string },
): PromotionEnvironment {
  return {
    activeRelease: null,
    proposedRelease: null,
    gates: [],
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
    ...overrides,
  };
}

const emptyFleet: FleetData = { instances: [] };

const noop = (): void => undefined;

function renderGate(syncRetries: number | null): void {
  const promotion: PromotionData = {
    order: ["hi", "hp"],
    environments: {
      hi: env({ name: "hi", role: "hub", syncRetries }),
      hp: env({ name: "hp", role: "hub" }),
    },
    releases: [],
    releaseByDigest: {},
    frontier: null,
  };
  const model = buildMapModel(promotion, emptyFleet);
  render(
    <IntlProvider locale="en" defaultLocale="en">
      <RouterProvider
        history={createMemoryHistory({ path: "/", search: "?gate=hi-%3Ehp" })}
      >
        <MapDetails
          model={model}
          releaseByDigest={{}}
          selection={{ kind: "gate", id: "hi->hp" }}
          onClose={noop}
          onSelectBundle={noop}
          onSelectNode={noop}
        />
      </RouterProvider>
    </IntlProvider>,
  );
}

function makeAgent(overrides?: Partial<SdlcAgentMetrics>): SdlcAgentMetrics {
  return {
    name: "triage",
    env: "hi",
    replicasOn: 1,
    replicasTotal: 1,
    queueDepth: 0,
    itemsProcessed: 0,
    itemsFailed: 0,
    provisionSeconds: 0,
    series: {
      times: [],
      queueDepth: [],
      itemsProcessed: [],
      itemsFailed: [],
      provisionSeconds: [],
    },
    ...overrides,
  };
}

function renderNodeAgentsTab(
  agentsByEnv?: ReadonlyMap<string, readonly SdlcAgentMetrics[]>,
): void {
  const promotion: PromotionData = {
    order: ["hi"],
    environments: {
      hi: env({ name: "hi", role: "hub" }),
    },
    releases: [],
    releaseByDigest: {},
    frontier: null,
  };
  const model = buildMapModel(promotion, emptyFleet);
  render(
    <IntlProvider locale="en" defaultLocale="en">
      <RouterProvider
        history={createMemoryHistory({
          path: "/",
          search: "?node=hi&nodeTab=agents",
        })}
      >
        <MapDetails
          model={model}
          releaseByDigest={{}}
          selection={{ kind: "node", id: "hi" }}
          onClose={noop}
          onSelectBundle={noop}
          onSelectNode={noop}
          agentsByEnv={agentsByEnv}
        />
      </RouterProvider>
    </IntlProvider>,
  );
}

describe("MapDetails - per-cluster Agents tab", () => {
  // Verifies: SDLC-AGT-11.01
  it("lists the agents reporting for this cluster", () => {
    const agentsByEnv = new Map<string, readonly SdlcAgentMetrics[]>([
      [
        "hi",
        [
          makeAgent({ name: "triage", queueDepth: 3, itemsFailed: 1 }),
          makeAgent({ name: "spec-review" }),
        ],
      ],
    ]);
    renderNodeAgentsTab(agentsByEnv);
    expect(screen.getByText("triage")).toBeTruthy();
    expect(screen.getByText("spec-review")).toBeTruthy();
  });

  // Verifies: SDLC-AGT-11.02
  it("shows the empty state when no agents report for this cluster", () => {
    renderNodeAgentsTab(new Map<string, readonly SdlcAgentMetrics[]>());
    expect(
      screen.getByText("No SDLC agents are reporting on this cluster."),
    ).toBeTruthy();
  });

  it("shows the empty state when no agents plane data is available", () => {
    renderNodeAgentsTab(undefined);
    expect(
      screen.getByText("No SDLC agents are reporting on this cluster."),
    ).toBeTruthy();
  });
});

describe("MapDetails - gate sync attempts", () => {
  // Verifies: FLT-SYNC-04.01
  it("shows the Sync attempts row with the count when known", () => {
    renderGate(9);
    expect(screen.getByText("Sync attempts: 9")).toBeTruthy();
  });

  // Verifies: FLT-SYNC-04.02
  it("omits the Sync attempts row when the count is unknown", () => {
    renderGate(null);
    expect(screen.queryByText("Sync attempts")).toBeNull();
  });

  it("shows a genuine 0 rather than omitting it", () => {
    renderGate(0);
    expect(screen.getByText("Sync attempts: 0")).toBeTruthy();
  });
});
