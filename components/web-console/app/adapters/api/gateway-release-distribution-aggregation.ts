import type { OperationalMetric } from "@openshift-online/hypershell-operational-dashboard-ui";
import type { GatewayList, SDKClient } from "@openshift-online/hypershell-sdk";

const gatewayListPageSize = 100;
const unknownBucketKey = "unknown";

export function bucketGatewayVersion(
  gatewayVersion: string | null | undefined,
): string {
  if (gatewayVersion === null || gatewayVersion === undefined) {
    return unknownBucketKey;
  }

  const trimmedVersion = gatewayVersion.trim();
  return trimmedVersion.length === 0 ? unknownBucketKey : trimmedVersion;
}

function validateGatewayListPage(
  requestedPage: number,
  result: GatewayList,
): void {
  const expectedItems = Math.min(
    gatewayListPageSize,
    Math.max(0, result.total - (requestedPage - 1) * gatewayListPageSize),
  );

  if (
    result.page !== requestedPage ||
    result.total < 0 ||
    result.items.length > expectedItems
  ) {
    throw new Error("Gateway list response was inconsistent");
  }
}

export interface GatewayReleaseDistributionAggregate {
  releaseDistribution: Map<string, number>;
  total: number;
}

export async function aggregateGatewayReleaseDistribution(
  client: SDKClient,
  signal: AbortSignal | undefined,
): Promise<GatewayReleaseDistributionAggregate> {
  let page = 1;
  let total = 0;
  const releaseDistribution = new Map<string, number>();

  do {
    const result: GatewayList = await client.gateways.list(
      { orderBy: "name asc", page, size: gatewayListPageSize },
      { signal },
    );

    validateGatewayListPage(page, result);

    for (const gateway of result.items) {
      const release = bucketGatewayVersion(gateway.gateway_version);
      releaseDistribution.set(
        release,
        (releaseDistribution.get(release) ?? 0) + 1,
      );
    }

    total = result.total;
    page += 1;
  } while ((page - 1) * gatewayListPageSize < total);

  const aggregatedGateways = [...releaseDistribution.values()].reduce(
    (sum, count) => sum + count,
    0,
  );
  if (aggregatedGateways !== total) {
    throw new Error("Gateway list response was inconsistent");
  }

  return { releaseDistribution, total };
}

export function buildGatewayReleasesMetric(
  aggregate: GatewayReleaseDistributionAggregate,
): OperationalMetric {
  return {
    id: "gateway-releases",
    releaseDistribution: Object.fromEntries(
      aggregate.releaseDistribution.entries(),
    ),
    value: String(aggregate.total),
  };
}
