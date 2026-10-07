const gatewayListPageSize = 100;
const gatewayPhases = [
  "Pending",
  "Provisioning",
  "Running",
  "Degraded",
  "Failed",
] as const;

interface AuthorizedGateway {
  active_sandbox_count: number;
  phase: string;
}

interface AuthorizedGatewayList {
  items: AuthorizedGateway[];
  page: number;
  total: number;
}

export interface AuthorizedGatewayMetrics {
  activeSandboxes: number;
  counts: Record<string, number>;
  total: number;
}

function emptyGatewayCounts(): Record<string, number> {
  return Object.fromEntries(gatewayPhases.map((phase) => [phase, 0]));
}

function validateListPage(
  requestedPage: number,
  body: AuthorizedGatewayList,
): void {
  const expectedItems = Math.min(
    gatewayListPageSize,
    Math.max(0, body.total - (requestedPage - 1) * gatewayListPageSize),
  );

  if (
    body.page !== requestedPage ||
    body.total < 0 ||
    body.items.length > expectedItems
  ) {
    throw new Error("Authorized gateway list response was inconsistent");
  }
}

export async function queryAuthorizedGatewayMetrics(
  apiOrigin: string,
  accessToken: string | undefined,
  timeoutMs: number,
): Promise<AuthorizedGatewayMetrics> {
  const counts = emptyGatewayCounts();
  let activeSandboxes = 0;
  let listedGateways = 0;
  let total = 0;
  let page = 1;

  do {
    const url = new URL("/api/hypershell/v1/gateways", apiOrigin);
    url.searchParams.set("page", String(page));
    url.searchParams.set("size", String(gatewayListPageSize));

    const response = await fetch(url, {
      headers: accessToken ? { authorization: `Bearer ${accessToken}` } : {},
      signal: AbortSignal.timeout(timeoutMs),
    });
    if (!response.ok) {
      throw new Error(
        `Authorized gateway list request failed: ${String(response.status)}`,
      );
    }

    const body = (await response.json()) as AuthorizedGatewayList;
    validateListPage(page, body);

    for (const gateway of body.items) {
      const phase = gateway.phase || "other";
      counts[phase] = (counts[phase] ?? 0) + 1;
      activeSandboxes += gateway.active_sandbox_count;
      listedGateways += 1;
    }

    total = body.total;
    page += 1;
  } while ((page - 1) * gatewayListPageSize < total);

  if (listedGateways !== total) {
    throw new Error("Authorized gateway list response was inconsistent");
  }

  return { activeSandboxes, counts, total };
}
