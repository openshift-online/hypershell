import type { MetricsSource } from "./metrics-source.js";
import {
  applicationScalarQuery,
  applicationVectorQuery,
  queryPrometheusInstantScalar,
  queryPrometheusInstantVector,
} from "./prometheus-instant-query.js";

export const managedClustersTotalPromql = "hypershell_managed_clusters_total";
export const managedClustersCreatedLast30DaysPromql =
  "hypershell_managed_clusters_created_last_30_days_total";
export const managedClustersInventoryPromql =
  "hypershell_managed_clusters_inventory_total";
export const managedDatabasesTotalPromql = "hypershell_managed_databases_total";
export const managedDatabasesInventoryPromql =
  "hypershell_managed_databases_inventory_total";

const clusterInventoryGroupBy = ["status", "provider", "region"] as const;
const databaseInventoryGroupBy = ["status"] as const;

export interface ManagedClustersInventoryResponse {
  by_provider: Record<string, number>;
  by_region: Record<string, number>;
  by_status: Record<string, number>;
  created_last_30_days: number;
  total: number;
}

export interface ManagedDatabasesInventoryResponse {
  by_status: Record<string, number>;
  total: number;
}

export interface PlatformInventoryResponse {
  managed_clusters: ManagedClustersInventoryResponse;
  managed_databases: ManagedDatabasesInventoryResponse;
}

function incrementBucket(
  buckets: Record<string, number>,
  key: string,
  amount: number,
): void {
  buckets[key] = (buckets[key] ?? 0) + amount;
}

function formatPlacementLabel(region: string, provider: string): string {
  return `${region} (${provider})`;
}

export async function queryPlatformInventory(
  prometheusUrl: MetricsSource,
  timeoutMs: number,
  namespace?: string,
): Promise<PlatformInventoryResponse> {
  const [
    clusterTotal,
    clustersCreatedLast30Days,
    clusterInventory,
    databaseTotal,
    databaseInventory,
  ] = await Promise.all([
    queryPrometheusInstantScalar(
      prometheusUrl,
      applicationScalarQuery(managedClustersTotalPromql, namespace),
      timeoutMs,
    ),
    queryPrometheusInstantScalar(
      prometheusUrl,
      applicationScalarQuery(managedClustersCreatedLast30DaysPromql, namespace),
      timeoutMs,
    ),
    queryPrometheusInstantVector(
      prometheusUrl,
      applicationVectorQuery(
        managedClustersInventoryPromql,
        clusterInventoryGroupBy,
        namespace,
      ),
      timeoutMs,
    ),
    queryPrometheusInstantScalar(
      prometheusUrl,
      applicationScalarQuery(managedDatabasesTotalPromql, namespace),
      timeoutMs,
    ),
    queryPrometheusInstantVector(
      prometheusUrl,
      applicationVectorQuery(
        managedDatabasesInventoryPromql,
        databaseInventoryGroupBy,
        namespace,
      ),
      timeoutMs,
    ),
  ]);

  const byStatus: Record<string, number> = {};
  const byProvider: Record<string, number> = {};
  const byRegion: Record<string, number> = {};

  for (const sample of clusterInventory) {
    const status = sample.labels.status ?? "unknown";
    const provider = sample.labels.provider ?? "unknown";
    const region = sample.labels.region ?? "unknown";
    incrementBucket(byStatus, status, sample.value);
    incrementBucket(byProvider, provider, sample.value);
    incrementBucket(
      byRegion,
      formatPlacementLabel(region, provider),
      sample.value,
    );
  }

  const databaseByStatus: Record<string, number> = {};
  for (const sample of databaseInventory) {
    const status = sample.labels.status ?? "unknown";
    incrementBucket(databaseByStatus, status, sample.value);
  }

  return {
    managed_clusters: {
      by_provider: byProvider,
      by_region: byRegion,
      by_status: byStatus,
      created_last_30_days: clustersCreatedLast30Days,
      total: clusterTotal,
    },
    managed_databases: {
      by_status: databaseByStatus,
      total: databaseTotal,
    },
  };
}
