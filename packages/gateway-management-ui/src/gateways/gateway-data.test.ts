import { describe, expect, it } from "vitest";

import type { GatewayRecord } from "../application/gateway-types";
import { toGatewayConnection } from "./gateway-data";

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
    expect(
      toGatewayConnection(gateway(), "Localized hub cluster"),
    ).toMatchObject({
      clusterName: "Localized hub cluster",
      endpoint: "https://gateway.example.com:443",
      id: "gateway-1",
      name: "Team gateway",
      status: "Ready",
    });
  });

  it("keeps a returned cluster identifier for name resolution only", () => {
    expect(
      toGatewayConnection(
        gateway({ clusterId: "cluster-east" }),
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
