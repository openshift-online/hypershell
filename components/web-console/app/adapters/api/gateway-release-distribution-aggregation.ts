import type { OperationalMetric } from "@openshift-online/hypershell-operational-dashboard-ui";
import type {
  GatewayList,
  GatewayReleaseList,
  SDKClient,
} from "@openshift-online/hypershell-sdk";

const listPageSize = 100;
const unknownBucketKey = "unknown";

export function bucketReleaseId(releaseId: string | null | undefined): string {
  if (releaseId === null || releaseId === undefined) {
    return unknownBucketKey;
  }

  const trimmed = releaseId.trim();
  return trimmed.length === 0 ? unknownBucketKey : trimmed;
}

export function resolveReleaseLabel(
  releaseId: string,
  releaseNameById: ReadonlyMap<string, string>,
): string {
  if (releaseId === unknownBucketKey) {
    return unknownBucketKey;
  }

  const releaseName = releaseNameById.get(releaseId);
  if (releaseName !== undefined) {
    const trimmedName = releaseName.trim();
    if (trimmedName.length > 0) {
      return trimmedName;
    }
  }

  return releaseId;
}

function incrementBucket(buckets: Map<string, number>, key: string): void {
  buckets.set(key, (buckets.get(key) ?? 0) + 1);
}

function mergeBucketCount(
  buckets: Map<string, number>,
  key: string,
  count: number,
): void {
  buckets.set(key, (buckets.get(key) ?? 0) + count);
}

function bucketsToRecord(buckets: Map<string, number>): Record<string, number> {
  return Object.fromEntries(buckets.entries());
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

export interface GatewayReleaseDistributionAggregate {
  releaseDistribution: Map<string, number>;
  total: number;
}

async function aggregateGatewayReleaseIdBuckets(
  client: SDKClient,
  signal: AbortSignal | undefined,
): Promise<{ releaseIdBuckets: Map<string, number>; total: number }> {
  let page = 1;
  let total = 0;
  const releaseIdBuckets = new Map<string, number>();

  do {
    const result: GatewayList = await client.gateways.list(
      { orderBy: "name asc", page, size: listPageSize },
      { signal },
    );

    validateListPageConsistency("Gateway", page, result, listPageSize);

    for (const gateway of result.items) {
      incrementBucket(releaseIdBuckets, bucketReleaseId(gateway.release_id));
    }

    total = result.total;
    page += 1;
  } while ((page - 1) * listPageSize < total);

  const aggregatedItems = [...releaseIdBuckets.values()].reduce(
    (sum, count) => sum + count,
    0,
  );
  if (aggregatedItems !== total) {
    throw new Error("Gateway list response was inconsistent");
  }

  return { releaseIdBuckets, total };
}

async function loadGatewayReleaseNameMap(
  client: SDKClient,
  signal: AbortSignal | undefined,
): Promise<Map<string, string>> {
  let page = 1;
  let total = 0;
  const releaseNameById = new Map<string, string>();

  do {
    const result: GatewayReleaseList = await client.gatewayReleases.list(
      { orderBy: "name asc", page, size: listPageSize },
      { signal },
    );

    validateListPageConsistency("Gateway release", page, result, listPageSize);

    for (const release of result.items) {
      releaseNameById.set(release.id, release.name);
    }

    total = result.total;
    page += 1;
  } while ((page - 1) * listPageSize < total);

  return releaseNameById;
}

export async function aggregateGatewayReleaseDistribution(
  client: SDKClient,
  signal: AbortSignal | undefined,
): Promise<GatewayReleaseDistributionAggregate> {
  const [{ releaseIdBuckets, total }, releaseNameById] = await Promise.all([
    aggregateGatewayReleaseIdBuckets(client, signal),
    loadGatewayReleaseNameMap(client, signal),
  ]);

  const releaseDistribution = new Map<string, number>();
  for (const [releaseId, count] of releaseIdBuckets) {
    const label = resolveReleaseLabel(releaseId, releaseNameById);
    mergeBucketCount(releaseDistribution, label, count);
  }

  const aggregatedDistributionItems = [...releaseDistribution.values()].reduce(
    (sum, count) => sum + count,
    0,
  );
  if (aggregatedDistributionItems !== total) {
    throw new Error("Gateway list response was inconsistent");
  }

  return {
    releaseDistribution,
    total,
  };
}

export function buildGatewayReleasesMetric(
  aggregate: GatewayReleaseDistributionAggregate,
): OperationalMetric {
  return {
    id: "gateway-releases",
    releaseDistribution: bucketsToRecord(aggregate.releaseDistribution),
    value: String(aggregate.total),
  };
}
