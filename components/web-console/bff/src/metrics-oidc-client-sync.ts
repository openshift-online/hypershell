import type { MetricsSource } from "./metrics-source.js";
import { queryPrometheusInstantNumberAllowAbsent } from "./prometheus-instant-query.js";

export const oidcClientsSyncedPromql =
  "sum(max by (hypershell_cluster_id) (oidc_client_synced))";
export const oidcClientsExpectedPromql =
  "sum(max by (hypershell_cluster_id) (oidc_client_expected))";
export const oidcKeycloakConnectedPromql =
  "min(max by (hypershell_cluster_id) (oidc_client_keycloak_connected))";

export type KeycloakStatus = "connected" | "disconnected" | "unknown";

export interface OidcClientSyncMetrics {
  accuracy_percent: number | null;
  synced_clients: number | null;
  expected_clients: number | null;
  keycloak_status: KeycloakStatus;
}

async function optionalNumber(
  source: MetricsSource,
  query: string,
  timeoutMs: number,
): Promise<number | undefined> {
  try {
    return await queryPrometheusInstantNumberAllowAbsent(
      source,
      query,
      timeoutMs,
    );
  } catch {
    return undefined;
  }
}

export async function queryOidcClientSync(
  source: MetricsSource,
  timeoutMs: number,
): Promise<OidcClientSyncMetrics> {
  const [synced, expected, connected] = await Promise.all([
    optionalNumber(source, oidcClientsSyncedPromql, timeoutMs),
    optionalNumber(source, oidcClientsExpectedPromql, timeoutMs),
    optionalNumber(source, oidcKeycloakConnectedPromql, timeoutMs),
  ]);

  const keycloakStatus: KeycloakStatus =
    connected === undefined
      ? "unknown"
      : connected >= 1
        ? "connected"
        : "disconnected";
  const validCounts =
    synced !== undefined &&
    expected !== undefined &&
    Number.isInteger(synced) &&
    Number.isInteger(expected) &&
    expected > 0 &&
    synced <= expected;

  return {
    accuracy_percent:
      validCounts && keycloakStatus === "connected"
        ? (100 * synced) / expected
        : null,
    synced_clients: synced === undefined ? null : Math.round(synced),
    expected_clients: expected === undefined ? null : Math.round(expected),
    keycloak_status: keycloakStatus,
  };
}
