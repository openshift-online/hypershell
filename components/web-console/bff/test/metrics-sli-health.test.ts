import { createServer, type Server } from "node:http";

import { afterEach, describe, expect, it } from "vitest";

import {
  certificateSourceAvailablePromql,
  expiredCertificatesPromql,
  queryCertificateHealth,
  soonestCertificateExpiryPromql,
  unavailableCertificatesPromql,
  validCertificatesPromql,
} from "../src/metrics-certificate-health.js";
import {
  oidcClientsExpectedPromql,
  oidcClientsSyncedPromql,
  oidcKeycloakConnectedPromql,
  queryOidcClientSync,
} from "../src/metrics-oidc-client-sync.js";
import {
  podReadyPromql,
  podTotalPromql,
  queryResourceReadiness,
  sandboxReadyPromql,
  sandboxReadySourceAvailablePromql,
  sandboxTotalPromql,
} from "../src/metrics-resource-readiness.js";

function body(value?: string): string {
  return JSON.stringify({
    status: "success",
    data: {
      result:
        value === undefined
          ? []
          : [{ metric: {}, value: ["1704067200", value] }],
    },
  });
}

async function startStub(values: Map<string, string | undefined>): Promise<{
  server: Server;
  url: string;
}> {
  const server = createServer((request, response) => {
    const query = new URL(
      request.url ?? "",
      "http://127.0.0.1",
    ).searchParams.get("query");
    response.setHeader("content-type", "application/json");
    response.end(body(query === null ? undefined : values.get(query)));
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const address = server.address();
  if (address === null || typeof address === "string") {
    throw new Error("expected listener");
  }
  return { server, url: `http://127.0.0.1:${String(address.port)}` };
}

describe("SLI health metric queries", () => {
  const servers: Server[] = [];
  afterEach(() => {
    for (const server of servers.splice(0)) server.close();
  });

  it("maps readiness independently and computes unready resources only from complete sources", async () => {
    const stub = await startStub(
      new Map([
        [podReadyPromql, "8"],
        [podTotalPromql, "10"],
        [sandboxReadyPromql, "3"],
        [sandboxReadySourceAvailablePromql, "1"],
        [sandboxTotalPromql, "4"],
      ]),
    );
    servers.push(stub.server);
    await expect(queryResourceReadiness(stub.url, 5_000)).resolves.toEqual({
      pod_readiness_percent: 80,
      sandbox_readiness_percent: 75,
      unready_resources: 3,
    });

    const partial = await startStub(
      new Map([
        [podReadyPromql, "8"],
        [podTotalPromql, "10"],
      ]),
    );
    servers.push(partial.server);
    await expect(queryResourceReadiness(partial.url, 5_000)).resolves.toEqual({
      pod_readiness_percent: 80,
    });

    const staleSandbox = await startStub(
      new Map([
        [podReadyPromql, "8"],
        [podTotalPromql, "10"],
        [sandboxReadyPromql, "4"],
        [sandboxTotalPromql, "4"],
        [sandboxReadySourceAvailablePromql, "0"],
      ]),
    );
    servers.push(staleSandbox.server);
    await expect(
      queryResourceReadiness(staleSandbox.url, 5_000),
    ).resolves.toEqual({ pod_readiness_percent: 80 });
  });

  it("uses the fleet minimum certificate expiry and preserves unavailable counts", async () => {
    const stub = await startStub(
      new Map([
        [validCertificatesPromql, "7"],
        [expiredCertificatesPromql, "1"],
        [unavailableCertificatesPromql, "2"],
        [soonestCertificateExpiryPromql, "5.5"],
        [certificateSourceAvailablePromql, "1"],
      ]),
    );
    servers.push(stub.server);
    expect(soonestCertificateExpiryPromql).toContain("min(max by");
    await expect(queryCertificateHealth(stub.url, 5_000)).resolves.toEqual({
      valid_certificates: 7,
      total_certificates: 10,
      expired_certificates: 1,
      status_unavailable: 2,
      soonest_days_to_expiry: 5.5,
    });
  });

  it("rejects empty or stale certificate populations", async () => {
    const empty = await startStub(
      new Map([
        [validCertificatesPromql, "0"],
        [expiredCertificatesPromql, "0"],
        [unavailableCertificatesPromql, "0"],
        [certificateSourceAvailablePromql, "1"],
      ]),
    );
    servers.push(empty.server);
    await expect(queryCertificateHealth(empty.url, 5_000)).rejects.toThrow(
      "Certificate health source is unavailable",
    );

    const stale = await startStub(
      new Map([
        [validCertificatesPromql, "7"],
        [expiredCertificatesPromql, "0"],
        [unavailableCertificatesPromql, "0"],
        [certificateSourceAvailablePromql, "0"],
      ]),
    );
    servers.push(stale.server);
    await expect(queryCertificateHealth(stale.url, 5_000)).rejects.toThrow(
      "Certificate health source is unavailable",
    );
  });

  it("does not synthesize OIDC accuracy for empty or disconnected populations", async () => {
    const connected = await startStub(
      new Map([
        [oidcClientsSyncedPromql, "3"],
        [oidcClientsExpectedPromql, "4"],
        [oidcKeycloakConnectedPromql, "1"],
      ]),
    );
    servers.push(connected.server);
    await expect(queryOidcClientSync(connected.url, 5_000)).resolves.toEqual({
      accuracy_percent: 75,
      synced_clients: 3,
      expected_clients: 4,
      keycloak_status: "connected",
    });

    const empty = await startStub(
      new Map([
        [oidcClientsSyncedPromql, "0"],
        [oidcClientsExpectedPromql, "0"],
        [oidcKeycloakConnectedPromql, "0"],
      ]),
    );
    servers.push(empty.server);
    await expect(queryOidcClientSync(empty.url, 5_000)).resolves.toEqual({
      accuracy_percent: null,
      synced_clients: 0,
      expected_clients: 0,
      keycloak_status: "disconnected",
    });
  });
});
