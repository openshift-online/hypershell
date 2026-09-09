import type { OperationalMetric } from "@openshift-online/hypershell-operational-dashboard-ui";
import type {
  ManagedClusterList,
  ManagedDatabaseList,
  SDKClient,
} from "@openshift-online/hypershell-sdk";

const inventoryListPageSize = 100;
const lookbackMilliseconds = 30 * 24 * 60 * 60 * 1000;
const unknownBucketKey = "unknown";

export function bucketInventoryField(value: string | null | undefined): string {
  if (value === null || value === undefined) {
    return unknownBucketKey;
  }

  const trimmed = value.trim();
  return trimmed.length === 0 ? unknownBucketKey : trimmed;
}

export function formatInventoryPlacementLabel(
  region: string | null | undefined,
  provider: string | null | undefined,
): string {
  const regionLabel = bucketInventoryField(region);
  const providerLabel = bucketInventoryField(provider);

  return `${regionLabel} (${providerLabel})`;
}

function incrementBucket(buckets: Map<string, number>, key: string): void {
  buckets.set(key, (buckets.get(key) ?? 0) + 1);
}

function bucketsToRecord(buckets: Map<string, number>): Record<string, number> {
  return Object.fromEntries(buckets.entries());
}

export function countCreatedWithinLookback(
  createdAt: string | null | undefined,
  evaluationTime: Date,
): boolean {
  if (createdAt === null || createdAt === undefined) {
    return false;
  }

  const createdMs = Date.parse(createdAt);
  if (!Number.isFinite(createdMs)) {
    return false;
  }

  const windowStartMs = evaluationTime.getTime() - lookbackMilliseconds;
  return createdMs >= windowStartMs;
}

function validateListPageConsistency(
  resourceLabel: string,
  requestedPage: number,
  result: { items: readonly unknown[]; page: number; total: number },
  pageSize: number,
): void {
  if (
    result.page !== requestedPage ||
    result.total < 0 ||
    result.items.length >
      Math.max(
        0,
        Math.min(pageSize, result.total - (requestedPage - 1) * pageSize),
      )
  ) {
    throw new Error(`${resourceLabel} list response was inconsistent`);
  }
}

export interface ManagedClusterInventoryAggregate {
  createdLast30Days: number;
  placementBuckets: Map<string, number>;
  providerBuckets: Map<string, number>;
  statusBuckets: Map<string, number>;
  total: number;
}

export interface ManagedDatabaseInventoryAggregate {
  statusBuckets: Map<string, number>;
  total: number;
}

export async function aggregateManagedClusterList(
  client: SDKClient,
  signal: AbortSignal | undefined,
): Promise<ManagedClusterInventoryAggregate> {
  let page = 1;
  let total = 0;
  const statusBuckets = new Map<string, number>();
  const providerBuckets = new Map<string, number>();
  const placementBuckets = new Map<string, number>();
  let createdLast30Days = 0;
  const evaluationTime = new Date();

  do {
    const result: ManagedClusterList = await client.managedClusters.list(
      { orderBy: "name asc", page, size: inventoryListPageSize },
      { signal },
    );

    validateListPageConsistency(
      "Managed cluster",
      page,
      result,
      inventoryListPageSize,
    );

    for (const cluster of result.items) {
      incrementBucket(statusBuckets, bucketInventoryField(cluster.status));
      incrementBucket(providerBuckets, bucketInventoryField(cluster.provider));
      incrementBucket(
        placementBuckets,
        formatInventoryPlacementLabel(cluster.region, cluster.provider),
      );

      if (countCreatedWithinLookback(cluster.created_at, evaluationTime)) {
        createdLast30Days += 1;
      }
    }

    total = result.total;
    page += 1;
  } while ((page - 1) * inventoryListPageSize < total);

  const aggregatedItems = [...statusBuckets.values()].reduce(
    (sum, count) => sum + count,
    0,
  );
  if (aggregatedItems !== total) {
    throw new Error("Managed cluster list response was inconsistent");
  }

  return {
    createdLast30Days,
    placementBuckets,
    providerBuckets,
    statusBuckets,
    total,
  };
}

export async function aggregateManagedDatabaseList(
  client: SDKClient,
  signal: AbortSignal | undefined,
): Promise<ManagedDatabaseInventoryAggregate> {
  let page = 1;
  let total = 0;
  const statusBuckets = new Map<string, number>();

  do {
    const result: ManagedDatabaseList = await client.managedDatabases.list(
      { orderBy: "name asc", page, size: inventoryListPageSize },
      { signal },
    );

    validateListPageConsistency(
      "Managed database",
      page,
      result,
      inventoryListPageSize,
    );

    for (const database of result.items) {
      incrementBucket(statusBuckets, bucketInventoryField(database.status));
    }

    total = result.total;
    page += 1;
  } while ((page - 1) * inventoryListPageSize < total);

  const aggregatedItems = [...statusBuckets.values()].reduce(
    (sum, count) => sum + count,
    0,
  );
  if (aggregatedItems !== total) {
    throw new Error("Managed database list response was inconsistent");
  }

  return {
    statusBuckets,
    total,
  };
}

export function buildManagedClustersMetric(
  aggregate: ManagedClusterInventoryAggregate,
): OperationalMetric {
  return {
    createdLast30Days: String(aggregate.createdLast30Days),
    id: "managed-clusters",
    inventoryProviders: bucketsToRecord(aggregate.providerBuckets),
    inventoryRegions: bucketsToRecord(aggregate.placementBuckets),
    inventoryStatus: bucketsToRecord(aggregate.statusBuckets),
    value: String(aggregate.total),
  };
}

export function buildManagedDatabasesMetric(
  aggregate: ManagedDatabaseInventoryAggregate,
): OperationalMetric {
  return {
    id: "managed-databases",
    inventoryStatus: bucketsToRecord(aggregate.statusBuckets),
    value: String(aggregate.total),
  };
}
