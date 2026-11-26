import { render } from "@testing-library/react";

import type { MapNode } from "../../../domain/map/model";
import type { NodeBox } from "../../../domain/map/layout";
import { NODE_H, NODE_W } from "../../../domain/map/layout";
import { MapNodeCard } from "./map-node";

function node(): MapNode {
  return {
    id: "env-a",
    columnKey: "env-a",
    envLabel: null,
    laneKey: "none",
    isHub: true,
    gateNames: [],
    gateChecks: [],
    role: null,
    provider: null,
    cluster: null,
    physicalClusterKey: null,
    colocatedWith: null,
    fromTopology: false,
    deliveryUnknown: false,
    seed: "sha256:a",
    version: null,
    digest: null,
    proposedVersion: null,
    proposedDigest: null,
    state: "behind",
    upToDate: false,
    driftsFromColumn: false,
    argoHealth: null,
    argoSync: null,
    syncRetries: null,
    prState: null,
    gateBadge: { tone: "unknown", labelKey: "unknown" },
    gateways: { running: 2 },
    gatewaysTotal: 2,
    gatewaysByCluster: [],
    spokeAttribution: null,
    gatewayTone: "unknown",
    gatewayHistory: [
      { running: 1, provisioning: 0, failed: 0 },
      { running: 2, provisioning: 0, failed: 0 },
    ],
    sandboxes: 3,
    sandboxesByCluster: [],
    sandboxHistory: [1, 3],
    logins: null,
    userHistory: [],
    loginsHistory: [],
    historyTimes: [],
    managedClusters: null,
    users: null,
    metrics: {
      rpc: { rate: 0, errorPct: 0, p95Ms: 0 },
      reconcile: { rate: 0, errorPct: 0, p95Ms: 0 },
      bff: { rate: 0, errorPct: 0, p95Ms: 0 },
      provisionP95Ms: null,
    },
    links: {
      console: null,
      grafana: null,
      argo: null,
      pr: null,
      analysis: null,
    },
  };
}

const BOX: NodeBox = {
  id: "env-a",
  x: 0,
  y: 0,
  w: NODE_W,
  h: NODE_H,
  cx: NODE_W / 2,
  cy: NODE_H / 2,
};

describe("MapNodeCard", () => {
  // Verifies: FLT-CAP-08.01
  it("draws both trend graphs right of the identity column, stacked", () => {
    const { container } = render(
      <svg>
        <MapNodeCard
          node={node()}
          box={BOX}
          selected={false}
          onSelect={() => undefined}
          onSelectBundle={() => undefined}
        />
      </svg>,
    );
    // Each sparkline clips to its own plot window: one rect per graph.
    const windows = [...container.querySelectorAll("rect")].filter(
      (r) => r.parentElement?.tagName.toLowerCase() === "clippath",
    );
    expect(windows).toHaveLength(2);
    const [gateways, sandboxes] = windows.map((r) => ({
      x: Number(r.getAttribute("x")),
      y: Number(r.getAttribute("y")),
      w: Number(r.getAttribute("width")),
    }));
    // Both start past the identity column (half the card) and share one x-axis.
    expect(gateways?.x).toBeGreaterThan(NODE_W / 3);
    expect(sandboxes?.x).toBe(gateways?.x);
    expect(sandboxes?.w).toBe(gateways?.w);
    // The sandbox graph sits below the gateway graph.
    expect(sandboxes?.y ?? 0).toBeGreaterThan(gateways?.y ?? 0);
  });
});
