import { describe, expect, it } from "vitest";

import { normalizeGatewayPlacementClusterIds } from "../application/gateway-placement";
import type { GatewayRecord } from "../application/gateway-types";
import {
  gatewayNeedsStatusPolling,
  gatewayPlacementBatchQueryKey,
  gatewayStatusPollMilliseconds,
  toGatewayConnection,
} from "./gateway-data";

function gateway(overrides: Partial<GatewayRecord> = {}): GatewayRecord {
  return {
    clusterId: "",
    createdAt: "2026-08-10T14:30:00Z",
    databaseId: "database-1",
    externalDns: "gateway.example.com",
    id: "gateway-1",
    name: "Team gateway",
    namespace: "openshell",
    phase: "",
    releaseId: "release-1",
    status: "Ready",
    ...overrides,
  };
}

describe("gateway presentation data", () => {
  it("uses one canonical cluster identifier normalization for batch keys", () => {
    const clusterIds = [" cluster-west ", "", "cluster-east", "cluster-west"];

    expect(normalizeGatewayPlacementClusterIds(clusterIds)).toEqual([
      "cluster-east",
      "cluster-west",
    ]);
    expect(gatewayPlacementBatchQueryKey(clusterIds)).toEqual([
      "gateways",
      "placements",
      "batch",
      "cluster-east",
      "cluster-west",
    ]);
  });

  it("maps gateway values into the connection view", () => {
    expect(
      toGatewayConnection(gateway(), "Localized hub cluster"),
    ).toMatchObject({
      clusterName: "Localized hub cluster",
      createdAt: "2026-08-10T14:30:00Z",
      endpoint: "https://gateway.example.com:443",
      id: "gateway-1",
      name: "Team gateway",
      status: "Ready",
    });
  });

  it("polls transitional gateway lifecycle states at a bounded interval", () => {
    expect(gatewayStatusPollMilliseconds).toBe(5_000);
    expect(gatewayNeedsStatusPolling(gateway({ phase: "", status: "" }))).toBe(
      true,
    );
    expect(gatewayNeedsStatusPolling(gateway({ phase: "Pending" }))).toBe(true);
    expect(gatewayNeedsStatusPolling(gateway({ phase: "Provisioning" }))).toBe(
      true,
    );
    expect(gatewayNeedsStatusPolling(gateway({ status: "Updating" }))).toBe(
      true,
    );
  });

  it("stops lifecycle polling for terminal gateway states", () => {
    expect(gatewayNeedsStatusPolling(gateway({ phase: "Running" }))).toBe(
      false,
    );
    expect(gatewayNeedsStatusPolling(gateway({ phase: "Failed" }))).toBe(false);
  });

  it("keeps a returned cluster identifier for name resolution only", () => {
    expect(
      toGatewayConnection(
        gateway({ clusterId: "  cluster-east  " }),
        "Hub cluster",
      ),
    ).toMatchObject({
      clusterId: "cluster-east",
      clusterName: "",
    });
  });

  it("keeps API-owned connection values unavailable when they are absent", () => {
    const connection = toGatewayConnection(
      gateway({ externalDns: undefined, status: undefined }),
      "Hub cluster",
    );

    expect(connection.endpoint).toBeUndefined();
    expect(connection.consoleUrl).toBeUndefined();
    expect(connection.oidcIssuer).toBeUndefined();
    expect(connection.status).toBe("Unknown");
  });
});
