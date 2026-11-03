import { render, screen } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { describe, expect, it } from "vitest";

import type { FleetData } from "../../../domain/fleet";
import type {
  PromotionData,
  PromotionEnvironment,
  PullRequest,
  ReleaseBundle,
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
  agentsPlaneStale?: boolean,
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
          agentsPlaneStale={agentsPlaneStale}
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

  // An idle cluster shows rows marked "Not active", distinct from the
  // never-reported empty state.
  // Verifies: SDLC-AGT-17.04
  it("shows Not active rows (not the empty state) for a cluster whose agents are idle", () => {
    const agentsByEnv = new Map<string, readonly SdlcAgentMetrics[]>([
      ["hi", [makeAgent({ name: "triage", replicasOn: 0 })]],
    ]);
    renderNodeAgentsTab(agentsByEnv, false);
    expect(screen.getByText("Not active")).toBeTruthy();
    expect(
      screen.queryByText("No SDLC agents are reporting on this cluster."),
    ).toBeNull();
  });

  // Active state on the Agents tab.
  // Verifies: SDLC-AGT-17.01
  it("marks a running agent Active", () => {
    const agentsByEnv = new Map<string, readonly SdlcAgentMetrics[]>([
      ["hi", [makeAgent({ name: "triage", replicasOn: 1 })]],
    ]);
    renderNodeAgentsTab(agentsByEnv, false);
    expect(screen.getByText("Active")).toBeTruthy();
  });

  // A stale plane reads as Monitoring blind on the Agents tab.
  // Verifies: SDLC-AGT-16.01, SDLC-AGT-17.01
  it("marks every agent Monitoring blind when the plane is stale", () => {
    const agentsByEnv = new Map<string, readonly SdlcAgentMetrics[]>([
      ["hi", [makeAgent({ name: "triage", replicasOn: 1 })]],
    ]);
    renderNodeAgentsTab(agentsByEnv, true);
    expect(screen.getByText("Monitoring blind")).toBeTruthy();
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

function renderNodeOverview(gatewayImage: string | null): void {
  const promotion: PromotionData = {
    order: ["hi"],
    environments: {
      hi: env({ name: "hi", role: "hub", gatewayImage }),
    },
    releases: [],
    releaseByDigest: {},
    frontier: null,
  };
  const model = buildMapModel(promotion, emptyFleet);
  render(
    <IntlProvider locale="en" defaultLocale="en">
      <RouterProvider
        history={createMemoryHistory({ path: "/", search: "?node=hi" })}
      >
        <MapDetails
          model={model}
          releaseByDigest={{}}
          selection={{ kind: "node", id: "hi" }}
          onClose={noop}
          onSelectBundle={noop}
          onSelectNode={noop}
        />
      </RouterProvider>
    </IntlProvider>,
  );
}

// Synthetic repo slugs: the fleet-identity firewall forbids the real GitOps slug in
// the public fleet-dashboard sources, including tests. Identity grouping is by the
// repository field, not any specific slug.
const APP_REPO = "acme/app";
const GITOPS_REPO = "acme/gitops-delivery";

function releaseBundle(overrides?: Partial<ReleaseBundle>): ReleaseBundle {
  return {
    version: "v1",
    digest: "sha256:abc",
    tag: null,
    date: null,
    prs: [],
    ...overrides,
  };
}

function renderBundle(bundle: ReleaseBundle): void {
  const digest = bundle.digest ?? bundle.version;
  const releaseByDigest: Record<string, ReleaseBundle> = { [digest]: bundle };
  const promotion: PromotionData = {
    order: [],
    environments: {},
    releases: [bundle.version],
    releaseByDigest,
    frontier: null,
  };
  const model = buildMapModel(promotion, emptyFleet);
  render(
    <IntlProvider locale="en" defaultLocale="en">
      <RouterProvider history={createMemoryHistory({ path: "/", search: "" })}>
        <MapDetails
          model={model}
          releaseByDigest={releaseByDigest}
          selection={{ kind: "bundle", id: digest }}
          onClose={noop}
          onSelectBundle={noop}
          onSelectNode={noop}
        />
      </RouterProvider>
    </IntlProvider>,
  );
}

function pr(repository: string, number: number): PullRequest {
  return {
    number,
    title: "change",
    url: `https://github.com/${repository}/pull/${String(number)}`,
    author: "a",
    mergedAt: null,
    repository,
  };
}

function prLinks(): HTMLAnchorElement[] {
  return Array.from(
    document.querySelectorAll<HTMLAnchorElement>("a[href*='/pull/']"),
  );
}

describe("MapDetails - bundle PRs by repository", () => {
  // Verifies: FLT-BPR-01.01, FLT-BPR-11.02
  it("shows application and GitOps groups each with its own PRs", () => {
    renderBundle(
      releaseBundle({
        prs: [pr(APP_REPO, 10)],
        gitopsPrs: [pr(GITOPS_REPO, 20)],
        gitopsStatus: "available",
      }),
    );
    expect(screen.getByText("In this bundle")).toBeTruthy();
    expect(screen.getByText("Deployment changes (GitOps)")).toBeTruthy();
    const hrefs = prLinks().map((a) => a.getAttribute("href"));
    expect(hrefs).toContain(`https://github.com/${APP_REPO}/pull/10`);
    expect(hrefs).toContain(`https://github.com/${GITOPS_REPO}/pull/20`);
  });

  // Verifies: FLT-BPR-02.01, FLT-BPR-11.01
  it("keeps the same PR number distinct across the two repositories", () => {
    renderBundle(
      releaseBundle({
        prs: [pr(APP_REPO, 42)],
        gitopsPrs: [pr(GITOPS_REPO, 42)],
        gitopsStatus: "available",
      }),
    );
    const hrefs = prLinks().map((a) => a.getAttribute("href"));
    // Both #42 rows are present, each linking to its own repository.
    expect(hrefs).toContain(`https://github.com/${APP_REPO}/pull/42`);
    expect(hrefs).toContain(`https://github.com/${GITOPS_REPO}/pull/42`);
    expect(prLinks()).toHaveLength(2);
  });

  // Verifies: FLT-BPR-10.02
  it("shows a distinct empty state for a successful empty GitOps range", () => {
    renderBundle(releaseBundle({ gitopsPrs: [], gitopsStatus: "available" }));
    expect(
      screen.getByText("No deployment changes in this range."),
    ).toBeTruthy();
    expect(
      screen.queryByText("Deployment changes are unavailable for this bundle."),
    ).toBeNull();
  });

  // Verifies: FLT-BPR-10.01, FLT-BPR-08.02
  it("shows an unavailable state, not empty, when the GitOps range is unavailable", () => {
    renderBundle(releaseBundle({ gitopsPrs: [], gitopsStatus: "unavailable" }));
    expect(
      screen.getByText("Deployment changes are unavailable for this bundle."),
    ).toBeTruthy();
    expect(
      screen.queryByText("No deployment changes in this range."),
    ).toBeNull();
  });

  // The GitOps section is omitted entirely when the server does not report it.
  it("omits the GitOps section when the server does not report GitOps grouping", () => {
    renderBundle(releaseBundle({ gitopsStatus: null }));
    expect(screen.getByText("In this bundle")).toBeTruthy();
    expect(screen.queryByText("Deployment changes (GitOps)")).toBeNull();
  });
});

describe("MapDetails - default gateway image", () => {
  // Verifies: FLT-CAP-07.01
  it("shows the image the control plane reports", () => {
    renderNodeOverview("quay.io/example/gateway:v0.1.5");
    expect(screen.getByText("Default gateway")).toBeTruthy();
    // Truncate (middle) splits the text across spans, so match the row's text.
    expect(document.body.textContent).toContain(
      "quay.io/example/gateway:v0.1.5",
    );
  });

  // Verifies: FLT-CAP-07.02
  it("omits the row when no image is reported", () => {
    renderNodeOverview(null);
    expect(screen.queryByText("Default gateway")).toBeNull();
  });
});
