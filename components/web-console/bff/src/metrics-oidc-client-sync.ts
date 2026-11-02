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
  const countsAvailable =
    keycloakStatus === "connected" &&
    synced !== undefined &&
    expected !== undefined &&
    Number.isInteger(synced) &&
    Number.isInteger(expected) &&
    synced >= 0 &&
    expected >= 0 &&
    synced <= expected;
  const validAccuracy = countsAvailable && expected > 0;

  return {
    accuracy_percent: validAccuracy ? (100 * synced) / expected : null,
    synced_clients: countsAvailable ? synced : null,
    expected_clients: countsAvailable ? expected : null,
    keycloak_status: keycloakStatus,
  };
}
