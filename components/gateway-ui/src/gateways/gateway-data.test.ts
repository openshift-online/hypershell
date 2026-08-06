import { describe, expect, it } from "vitest";

import { toGatewayConnection } from "./gateway-data";
import type { GatewayRecord } from "./gateway-types";

function gateway(overrides: Partial<GatewayRecord> = {}): GatewayRecord {
  return {
    clusterId: "",
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
  it("maps gateway values into the connection view", () => {
    expect(toGatewayConnection(gateway())).toMatchObject({
      clusterName: "Local cluster",
      endpoint: "https://gateway.example.com:443",
      id: "gateway-1",
      name: "Team gateway",
      status: "Ready",
    });
  });

  it("uses a returned cluster identifier when one is present", () => {
    expect(
      toGatewayConnection(gateway({ clusterId: "cluster-east" })).clusterName,
    ).toBe("cluster-east");
  });

  it("uses the development endpoint when optional DNS is absent", () => {
    const connection = toGatewayConnection(
      gateway({ externalDns: undefined, status: undefined }),
    );

    expect(connection.endpoint).toBe(
      "https://openshell-gw-openshell-gateway-test.apps.rosa.hcmais01ue1.s9m2.p3.openshiftapps.com:443",
    );
    expect(connection.status).toBe("Unknown");
  });
});
