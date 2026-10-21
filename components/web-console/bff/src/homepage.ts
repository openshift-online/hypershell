import { queryAuthorizedGatewayInventory } from "./authorized-gateway-metrics.js";
import { queryGatewaySandboxes } from "./metrics-gateway-sandboxes.js";

const homepageListPageSize = 100;
const clusterHeartbeatMaxAgeMs = 5 * 60 * 1000;

export interface HomepageAttentionEntry {
  gatewayId: string;
  name: string;
  reason: string;
  status: string;
}

export interface HomepageGatewayHealth {
  attention: readonly HomepageAttentionEntry[];
  attentionCount: number;
  healthy: number;
  total: number;
}

export interface HomepageSummary {
  gatewayHealth: HomepageGatewayHealth | { available: false };
}

export interface HomepageClusterAvailability {
  healthy: number;
  total: number;
  unhealthyClusters: readonly {
    name: string;
    reason: string;
    status: string;
  }[];
}

export interface HomepagePlatformOverview {
  clusters: HomepageClusterAvailability | { available: false };
  gatewayStatuses:
    | { available: true; counts: Record<string, number>; total: number }
    | { available: false };
  sandboxStatuses:
    | { available: true; counts: Record<string, number>; total: number }
    | { available: false };
}

interface GatewayListItem {
  id?: string;
  name?: string;
  phase?: string;
  status?: string;
}

interface ManagedClusterItem {
  id?: string;
  last_seen_at?: string;
  name?: string;
  status?: string;
}

interface ListPage<T> {
  items?: T[];
  page?: number;
  total?: number;
}

async function fetchJson<T>(
  url: URL,
  accessToken: string | undefined,
  timeoutMs: number,
): Promise<T> {
  const response = await fetch(url, {
    headers: accessToken ? { authorization: `Bearer ${accessToken}` } : {},
    signal: AbortSignal.timeout(timeoutMs),
  });
  if (!response.ok) {
    throw new Error(
      `Homepage source request failed: ${String(response.status)}`,
    );
  }
  return (await response.json()) as T;
}

function validateListPage<T>(
  page: ListPage<T>,
  requestedPage: number,
  expectedTotal: number | undefined,
): void {
  const total = page.total ?? 0;
  const expectedItems = Math.min(
    homepageListPageSize,
    Math.max(0, total - (requestedPage - 1) * homepageListPageSize),
  );
  if (
    page.page !== requestedPage ||
    total < 0 ||
    (page.items?.length ?? 0) !== expectedItems ||
    (expectedTotal !== undefined && total !== expectedTotal)
  ) {
    throw new Error("Homepage list response was inconsistent");
  }
}

async function listAll<T>(
  apiOrigin: string,
  pathname: string,
  accessToken: string | undefined,
  timeoutMs: number,
): Promise<readonly T[]> {
  const items: T[] = [];
  let page = 1;
  let total: number | undefined;
  for (;;) {
    const url = new URL(pathname, apiOrigin);
    url.searchParams.set("page", String(page));
    url.searchParams.set("size", String(homepageListPageSize));
    const body = await fetchJson<ListPage<T>>(url, accessToken, timeoutMs);
    validateListPage(body, page, total);
    items.push(...(body.items ?? []));
    total = body.total ?? 0;
    if (
      (body.items?.length ?? 0) < homepageListPageSize ||
      page * homepageListPageSize >= total
    ) {
      return items;
    }
    page += 1;
  }
}

async function queryGatewayHealth(
  apiOrigin: string,
  accessToken: string | undefined,
  timeoutMs: number,
): Promise<HomepageGatewayHealth> {
  const gateways = await listAll<GatewayListItem>(
    apiOrigin,
    "/api/hypershell/v1/gateways",
    accessToken,
    timeoutMs,
  );
  let healthy = 0;
  const attention: HomepageAttentionEntry[] = [];
  for (const gateway of gateways) {
    const phase = gateway.phase?.trim() ?? "Unknown";
    const status = gateway.status?.trim() ?? phase;
    if (phase === "Running") {
      healthy += 1;
      continue;
    }
    attention.push({
      gatewayId: gateway.id ?? "",
      name: gateway.name ?? gateway.id ?? "",
      reason: gateway.status?.trim() ?? "Gateway is not running",
      status,
    });
  }
  return {
    attention,
    attentionCount: attention.length,
    healthy,
    total: gateways.length,
  };
}

async function queryClusterAvailability(
  apiOrigin: string,
  accessToken: string | undefined,
  timeoutMs: number,
): Promise<HomepageClusterAvailability> {
  const clusters = await listAll<ManagedClusterItem>(
    apiOrigin,
    "/api/hypershell/v1/managed_clusters",
    accessToken,
    timeoutMs,
  );
  const now = Date.now();
  let healthy = 0;
  const unhealthyClusters: {
    name: string;
    reason: string;
    status: string;
  }[] = [];
  for (const cluster of clusters) {
    const lastSeen = cluster.last_seen_at
      ? Date.parse(cluster.last_seen_at)
      : NaN;
    const age = now - lastSeen;
    const fresh =
      Number.isFinite(age) && age >= 0 && age < clusterHeartbeatMaxAgeMs;
    const status = cluster.status?.trim() ?? "Unknown";
    if (fresh) {
      healthy += 1;
      continue;
    }
    unhealthyClusters.push({
      name: cluster.name ?? cluster.id ?? "",
      reason: "Cluster heartbeat is stale",
      status,
    });
  }
  return { healthy, total: clusters.length, unhealthyClusters };
}

export async function queryHomepageSummary(
  apiOrigin: string,
  accessToken: string | undefined,
  timeoutMs: number,
): Promise<HomepageSummary> {
  try {
    return {
      gatewayHealth: await queryGatewayHealth(
        apiOrigin,
        accessToken,
        timeoutMs,
      ),
    };
  } catch {
    return {
      gatewayHealth: { available: false },
    };
  }
}

export async function queryHomepagePlatformOverview(
  apiOrigin: string,
  accessToken: string | undefined,
  apiTimeoutMs: number,
  prometheusUrl: string,
  prometheusTimeoutMs: number,
  prometheusNamespace?: string,
): Promise<HomepagePlatformOverview> {
  const [gateways, sandboxes, clusters] = await Promise.allSettled([
    queryAuthorizedGatewayInventory(apiOrigin, accessToken, apiTimeoutMs),
    queryGatewaySandboxes(
      prometheusUrl,
      prometheusTimeoutMs,
      prometheusNamespace,
    ),
    queryClusterAvailability(apiOrigin, accessToken, apiTimeoutMs),
  ]);

  return {
    clusters:
      clusters.status === "fulfilled" ? clusters.value : { available: false },
    gatewayStatuses:
      gateways.status === "fulfilled"
        ? {
            available: true,
            counts: gateways.value.counts,
            total: gateways.value.total,
          }
        : { available: false },
    sandboxStatuses:
      sandboxes.status === "fulfilled"
        ? {
            available: true,
            counts: {
              active: sandboxes.value.active_sandboxes,
              ...(sandboxes.value.expiring_sandboxes !== undefined
                ? { expiring: sandboxes.value.expiring_sandboxes }
                : {}),
              ...(sandboxes.value.idle_sandboxes !== undefined
                ? { idle: sandboxes.value.idle_sandboxes }
                : {}),
              ...(sandboxes.value.orphaned_sandboxes !== undefined
                ? { orphaned: sandboxes.value.orphaned_sandboxes }
                : {}),
            },
            total: sandboxes.value.active_sandboxes,
          }
        : { available: false },
  };
}
