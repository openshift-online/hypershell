import type {
  CostDashboardMetrics,
  CostTrendPoint,
} from "../application/cost-dashboard-types";

function dailyPoints(values: readonly (number | null)[]): CostTrendPoint[] {
  const end = new Date();
  const endDate = Date.UTC(
    end.getUTCFullYear(),
    end.getUTCMonth(),
    end.getUTCDate(),
  );
  return values.map((value, index) => ({
    label: new Date(endDate - (values.length - 1 - index) * 86_400_000)
      .toISOString()
      .slice(0, 10),
    value,
  }));
}

/** Platform view fixture; numbers match the approved cost mockup. */
export const mockCostDashboardMetrics: CostDashboardMetrics = {
  currency: "USD",
  lastSuccessfulRefresh: new Date(),
  platform: {
    summary: {
      idleMonthly: 2180,
      idlePercentOfTotal: 12,
      idleTrend: dailyPoints([62, 66, 70, 71, 69, 74, 72]),
      monthlyTotal: 18420,
      perActiveUser: 46.05,
      perGateway: 189.9,
      perSandbox: 54.18,
      unusedGatewaysMonthly: 410,
    },
    breakdown: {
      byBillingOwner: [
        { gatewayCount: 6, key: "alice", monthly: 1320 },
        { gatewayCount: 4, key: "bob", monthly: 860 },
        { gatewayCount: 3, key: "carol", monthly: 540 },
      ],
      byCluster: [
        {
          environment: "production",
          key: "Cluster alpha",
          monthly: 8420,
          provider: "ibm",
          trend: dailyPoints([270, 272, 276, 275, 279, 281, 283]),
        },
        {
          environment: "production",
          key: "Cluster beta",
          monthly: 5180,
          provider: "aws",
          trend: dailyPoints([166, 168, 170, 169, 171, 172, 174]),
        },
      ],
      byEnvironment: [
        {
          key: "Production",
          monthly: 11240,
          trend: dailyPoints([360, 365, 371, 368, 374, 379, 383]),
        },
        {
          key: "Staging",
          monthly: 4810,
          trend: dailyPoints([150, 152, 158, 160, 157, 161, 164]),
        },
        {
          key: "Development",
          monthly: 2370,
          trend: dailyPoints([75, 76, 80, 78, 79, 81, 83]),
        },
      ],
      byResourceClass: {
        gateways: 9820,
        idle: 2180,
        platformOverhead: 0,
        sandboxes: 6440,
      },
    },
    topGateways: [
      {
        billingOwner: "alice",
        cluster: "Cluster alpha",
        id: "gw-1",
        monthly: 612,
        name: "payments-agents",
      },
      {
        billingOwner: "bob",
        cluster: "Cluster beta",
        id: "gw-2",
        monthly: 410,
        name: "ci-sandbox",
        unused: true,
      },
    ],
  },
  rateCardVersion: "2026-11-26",
  status: "ok",
  timeRange: 7,
  view: "platform",
};

/** My cost fixture; numbers match the My cost mockup. */
export const mockMyCostDashboardMetrics: CostDashboardMetrics = {
  currency: "USD",
  empty: false,
  lastSuccessfulRefresh: new Date(),
  mine: {
    gateways: [
      {
        billingOwner: "you",
        charged: true,
        cluster: "Cluster alpha",
        id: "gw-1",
        monthly: 612,
        name: "payments-agents",
        role: "owner",
      },
      {
        billingOwner: "you",
        charged: true,
        cluster: "Cluster beta",
        id: "gw-3",
        monthly: 128.4,
        name: "docs-bot",
        role: "owner",
        unused: true,
      },
      {
        billingOwner: "dana",
        charged: false,
        cluster: "Cluster alpha",
        id: "gw-4",
        monthly: 287.5,
        name: "team-research",
        role: "user",
      },
    ],
    summary: {
      chargedGatewayCount: 2,
      chargedMonthly: 740.4,
      sharedGatewayCount: 1,
      sharedMonthly: 287.5,
      trend: dailyPoints([23.1, 24, 24.4, 25, 24.2, 24.8, 25.3]),
    },
    workspaces: [
      {
        gatewayId: "gw-1",
        gatewayName: "payments-agents",
        monthly: 438.2,
        workspace: "checkout",
      },
      {
        gatewayId: "gw-1",
        gatewayName: "payments-agents",
        monthly: 121.6,
        workspace: "refunds",
      },
    ],
  },
  rateCardVersion: "2026-11-26",
  status: "ok",
  timeRange: 7,
  view: "mine",
};

export const mockCostDashboardNotConfiguredMetrics: CostDashboardMetrics = {
  lastSuccessfulRefresh: new Date(),
  status: "not-configured",
  view: "platform",
};
