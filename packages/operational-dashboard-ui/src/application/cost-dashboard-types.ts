import type { DashboardTimeRange } from "./dashboard-types";

/** Same-origin BFF sources behind the cost dashboard (WEB-COST "Metric sources"). */
export type CostMetricSourceId =
  | "cost-summary"
  | "cost-breakdown"
  | "cost-gateways"
  | "my-cost-summary"
  | "my-cost-gateways";

export type CostMetricId =
  | "cost-total"
  | "cost-idle"
  | "cost-per-user"
  | "cost-per-gateway"
  | "cost-per-sandbox"
  | "cost-by-environment"
  | "cost-by-cluster"
  | "cost-by-resource-class"
  | "cost-by-billing-owner"
  | "cost-top-gateways"
  | "my-cost-charged"
  | "my-cost-shared"
  | "my-cost-trend"
  | "my-cost-by-gateway"
  | "my-cost-by-workspace";

/** Which view of the cost dashboard is loaded. */
export type CostDashboardView = "platform" | "mine";

export interface CostTrendPoint {
  label: string;
  value: number | null;
}

export interface CostAmountRow {
  key: string;
  /** Estimated cost per 730-hour month, in `currency`. */
  monthly: number;
  trend?: readonly CostTrendPoint[];
}

export interface CostPlatformSummary {
  idleMonthly: number;
  idlePercentOfTotal: number;
  idleTrend?: readonly CostTrendPoint[];
  monthlyTotal: number;
  perActiveUser?: number;
  perGateway?: number;
  perSandbox?: number;
  unusedGatewaysMonthly?: number;
}

export interface CostPlatformBreakdown {
  byBillingOwner?: readonly (CostAmountRow & { gatewayCount: number })[];
  byCluster: readonly (CostAmountRow & {
    environment: string;
    provider: string;
  })[];
  byEnvironment: readonly CostAmountRow[];
  byResourceClass: {
    gateways: number;
    idle: number;
    platformOverhead: number;
    sandboxes: number;
  };
}

export interface CostGatewayRow {
  billingOwner?: string;
  cluster: string;
  id: string;
  monthly: number;
  name: string;
  unused?: boolean;
}

export type CostGatewayRole = "owner" | "admin" | "user";

export interface MyCostGatewayRow extends CostGatewayRow {
  /** True when the signed-in user is this gateway's billing owner. */
  charged: boolean;
  role: CostGatewayRole;
}

export interface MyCostWorkspaceRow {
  gatewayId: string;
  gatewayName: string;
  monthly: number;
  workspace: string;
}

export interface MyCostSummary {
  chargedGatewayCount: number;
  chargedMonthly: number;
  sharedGatewayCount: number;
  sharedMonthly: number;
  trend: readonly CostTrendPoint[];
}

export interface CostDashboardMetrics {
  coverageEnd?: string;
  coverageStart?: string;
  currency?: string;
  /** True when the signed-in user holds no gateway binding (WEB-COST-09.03). */
  empty?: boolean;
  failedSources?: readonly CostMetricSourceId[];
  lastSuccessfulRefresh: Date;
  mine?: {
    gateways?: readonly MyCostGatewayRow[];
    summary?: MyCostSummary;
    workspaces?: readonly MyCostWorkspaceRow[];
  };
  platform?: {
    breakdown?: CostPlatformBreakdown;
    summary?: CostPlatformSummary;
    topGateways?: readonly CostGatewayRow[];
  };
  rateCardVersion?: string;
  /** "not-configured" when the deployment supplies no rate card (WEB-COST-03). */
  status: "ok" | "not-configured";
  timeRange?: DashboardTimeRange;
  unpricedClusters?: readonly string[];
  view: CostDashboardView;
}
