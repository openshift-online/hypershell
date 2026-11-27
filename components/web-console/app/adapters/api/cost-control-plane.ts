import type {
  CostDashboardMetrics,
  CostMetricSourceId,
  CostTrendPoint,
  DashboardInvocationContext,
  MyCostGatewayRow,
} from "@openshift-online/hypershell-operational-dashboard-ui";

/**
 * Browser adapter for the cost dashboard BFF routes (WEB-COST "Metric
 * sources"). The browser never sees the rate card: responses carry only the
 * currency, rate card version, and priced amounts.
 */

interface DailyPoint {
  date: string;
  value: number | null;
}

interface Envelope {
  coverage_end?: string;
  coverage_start?: string;
  currency?: string;
  rate_card_version?: string;
  status?: "ok" | "not-configured";
  unpriced_clusters?: string[];
}

interface SummaryBody extends Envelope {
  idle: {
    daily?: DailyPoint[];
    monthly: number;
    percent_of_total: number;
    unused_gateways_monthly?: number;
  };
  monthly_total: number;
  per_active_user?: number;
  per_gateway?: number;
  per_sandbox?: number;
}

interface BreakdownBody extends Envelope {
  by_billing_owner?: {
    gateway_count: number;
    monthly: number;
    username: string;
  }[];
  by_cluster: {
    daily?: DailyPoint[];
    environment: string;
    key: string;
    monthly: number;
    provider: string;
  }[];
  by_environment: { daily?: DailyPoint[]; key: string; monthly: number }[];
  by_resource_class: {
    gateways: number;
    idle: number;
    platform_overhead: number;
    sandboxes: number;
  };
}

interface GatewaysBody extends Envelope {
  gateways: {
    billing_owner?: string;
    cluster: string;
    id: string;
    monthly: number;
    name: string;
    unused?: boolean;
  }[];
}

interface MySummaryBody extends Envelope {
  charged: { gateway_count: number; monthly: number };
  empty?: boolean;
  shared: { gateway_count: number; monthly: number };
  trend: DailyPoint[];
}

interface MyGatewaysBody extends Envelope {
  empty?: boolean;
  gateways: {
    billing_owner?: string;
    charged: boolean;
    cluster: string;
    id: string;
    monthly: number;
    name: string;
    role: MyCostGatewayRow["role"];
    unused?: boolean;
  }[];
  workspaces: {
    gateway_id: string;
    gateway_name: string;
    monthly: number;
    workspace: string;
  }[];
}

function amount(value: unknown, field: string): number {
  if (typeof value !== "number" || !Number.isFinite(value) || value < 0) {
    throw new Error(`Invalid cost response field: ${field}`);
  }
  return value;
}

function optionalAmount(value: unknown, field: string): number | undefined {
  return value === undefined ? undefined : amount(value, field);
}

function trend(points: readonly DailyPoint[] | undefined) {
  return points?.map((point): CostTrendPoint => ({
    label: point.date,
    value: point.value,
  }));
}

async function fetchSource<T extends Envelope>(
  context: DashboardInvocationContext,
  path: string,
): Promise<T> {
  const response = await fetch(
    `${path}?range=${String(context.timeRange ?? 7)}`,
    {
      credentials: "same-origin",
      signal: context.signal,
    },
  );
  if (!response.ok) {
    throw new Error(`Failed to fetch cost metrics: ${String(response.status)}`);
  }
  return (await response.json()) as T;
}

function isAbortError(error: unknown): boolean {
  return (
    typeof error === "object" &&
    error !== null &&
    "name" in error &&
    error.name === "AbortError"
  );
}

type Settled<T> = { ok: true; value: T } | { ok: false };

async function settle<T>(promise: Promise<T>): Promise<Settled<T>> {
  try {
    return { ok: true, value: await promise };
  } catch (error) {
    if (isAbortError(error)) throw error;
    return { ok: false };
  }
}

function envelopeOf(
  bodies: readonly (Envelope | undefined)[],
): Pick<
  CostDashboardMetrics,
  | "coverageEnd"
  | "coverageStart"
  | "currency"
  | "rateCardVersion"
  | "status"
  | "unpricedClusters"
> {
  const present = bodies.filter((body): body is Envelope => body !== undefined);
  const notConfigured = present.some(
    (body) => body.status === "not-configured",
  );
  const first = present.find((body) => body.status === "ok");
  const unpriced = new Set(
    present.flatMap((body) => body.unpriced_clusters ?? []),
  );
  return {
    status: notConfigured ? "not-configured" : "ok",
    ...(first?.currency ? { currency: first.currency } : {}),
    ...(first?.rate_card_version
      ? { rateCardVersion: first.rate_card_version }
      : {}),
    ...(first?.coverage_start
      ? {
          coverageStart: first.coverage_start,
          coverageEnd: first.coverage_end,
        }
      : {}),
    ...(unpriced.size > 0 ? { unpricedClusters: [...unpriced].sort() } : {}),
  };
}

const okBody = <T extends Envelope>(result: Settled<T>) =>
  result.ok && result.value.status === "ok" ? result.value : undefined;

export async function fetchCostDashboardMetrics(
  context: DashboardInvocationContext,
): Promise<CostDashboardMetrics> {
  context.signal?.throwIfAborted();
  const timeRange = context.timeRange ?? 7;

  if (context.costView === "platform") {
    const [summary, breakdown, gateways] = await Promise.all([
      settle(fetchSource<SummaryBody>(context, "/api/metrics/cost/summary")),
      settle(
        fetchSource<BreakdownBody>(context, "/api/metrics/cost/breakdown"),
      ),
      settle(fetchSource<GatewaysBody>(context, "/api/metrics/cost/gateways")),
    ]);
    const failedSources: CostMetricSourceId[] = [];
    if (!summary.ok) failedSources.push("cost-summary");
    if (!breakdown.ok) failedSources.push("cost-breakdown");
    if (!gateways.ok) failedSources.push("cost-gateways");
    if (failedSources.length === 3) {
      throw new Error("All cost dashboard metric sources failed");
    }
    const s = okBody(summary);
    const b = okBody(breakdown);
    const g = okBody(gateways);
    return {
      ...envelopeOf([
        summary.ok ? summary.value : undefined,
        breakdown.ok ? breakdown.value : undefined,
        gateways.ok ? gateways.value : undefined,
      ]),
      ...(failedSources.length > 0 ? { failedSources } : {}),
      lastSuccessfulRefresh: new Date(),
      platform: {
        summary: s && {
          idleMonthly: amount(s.idle.monthly, "idle.monthly"),
          idlePercentOfTotal: amount(
            s.idle.percent_of_total,
            "idle.percent_of_total",
          ),
          idleTrend: trend(s.idle.daily),
          monthlyTotal: amount(s.monthly_total, "monthly_total"),
          perActiveUser: optionalAmount(s.per_active_user, "per_active_user"),
          perGateway: optionalAmount(s.per_gateway, "per_gateway"),
          perSandbox: optionalAmount(s.per_sandbox, "per_sandbox"),
          unusedGatewaysMonthly: optionalAmount(
            s.idle.unused_gateways_monthly,
            "idle.unused_gateways_monthly",
          ),
        },
        breakdown: b && {
          byBillingOwner: b.by_billing_owner?.map((row) => ({
            gatewayCount: row.gateway_count,
            key: row.username,
            monthly: amount(row.monthly, "by_billing_owner.monthly"),
          })),
          byCluster: b.by_cluster.map((row) => ({
            environment: row.environment,
            key: row.key,
            monthly: amount(row.monthly, "by_cluster.monthly"),
            provider: row.provider,
            trend: trend(row.daily),
          })),
          byEnvironment: b.by_environment.map((row) => ({
            key: row.key,
            monthly: amount(row.monthly, "by_environment.monthly"),
            trend: trend(row.daily),
          })),
          byResourceClass: {
            gateways: amount(b.by_resource_class.gateways, "gateways"),
            idle: amount(b.by_resource_class.idle, "idle"),
            platformOverhead: amount(
              b.by_resource_class.platform_overhead,
              "platform_overhead",
            ),
            sandboxes: amount(b.by_resource_class.sandboxes, "sandboxes"),
          },
        },
        topGateways: g?.gateways.map((row) => ({
          billingOwner: row.billing_owner,
          cluster: row.cluster,
          id: row.id,
          monthly: amount(row.monthly, "gateways.monthly"),
          name: row.name,
          unused: row.unused,
        })),
      },
      timeRange,
      view: "platform",
    };
  }

  const [summary, gateways] = await Promise.all([
    settle(fetchSource<MySummaryBody>(context, "/api/metrics/cost/me/summary")),
    settle(
      fetchSource<MyGatewaysBody>(context, "/api/metrics/cost/me/gateways"),
    ),
  ]);
  const failedSources: CostMetricSourceId[] = [];
  if (!summary.ok) failedSources.push("my-cost-summary");
  if (!gateways.ok) failedSources.push("my-cost-gateways");
  if (failedSources.length === 2) {
    throw new Error("All cost dashboard metric sources failed");
  }
  const s = okBody(summary);
  const g = okBody(gateways);
  return {
    ...envelopeOf([
      summary.ok ? summary.value : undefined,
      gateways.ok ? gateways.value : undefined,
    ]),
    ...(failedSources.length > 0 ? { failedSources } : {}),
    empty: (s?.empty ?? g?.empty) === true,
    lastSuccessfulRefresh: new Date(),
    mine: {
      summary: s && {
        chargedGatewayCount: s.charged.gateway_count,
        chargedMonthly: amount(s.charged.monthly, "charged.monthly"),
        sharedGatewayCount: s.shared.gateway_count,
        sharedMonthly: amount(s.shared.monthly, "shared.monthly"),
        trend: trend(s.trend) ?? [],
      },
      gateways: g?.gateways.map((row) => ({
        billingOwner: row.billing_owner,
        charged: row.charged,
        cluster: row.cluster,
        id: row.id,
        monthly: amount(row.monthly, "gateways.monthly"),
        name: row.name,
        role: row.role,
        unused: row.unused,
      })),
      workspaces: g?.workspaces.map((row) => ({
        gatewayId: row.gateway_id,
        gatewayName: row.gateway_name,
        monthly: amount(row.monthly, "workspaces.monthly"),
        workspace: row.workspace,
      })),
    },
    timeRange,
    view: "mine",
  };
}
