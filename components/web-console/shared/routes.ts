export const routes = {
  adminPlatformPolicies: "admin/platform-policies",
  adminPlatformPolicyDetail: "admin/platform-policies/:policyId",
  adminPlatformPolicyEdit: "admin/platform-policies/:policyId/edit",
  adminPlatformPolicyNew: "admin/platform-policies/new",
  dashboard: "dashboard",
  dashboardAdoption: "dashboard/adoption",
  dashboardCost: "dashboard/cost",
  dashboardInventory: "dashboard/inventory",
  dashboardProvisioning: "dashboard/provisioning",
  dashboardReliability: "dashboard/reliability",
  dashboardUsage: "dashboard/usage",
  gatewayDetail: "gateways/:gatewayId",
  gatewayList: "gateways",
  gatewayNew: "gateways/new",
  home: "/",
  login: "login",
  metrics: "metrics",
} as const;

export type RouteName = keyof typeof routes;

function routePath(name: RouteName): string {
  const value = routes[name];
  return value.startsWith("/") ? value : `/${value}`;
}

export const routePaths = {
  adminPlatformPolicies: routePath("adminPlatformPolicies"),
  adminPlatformPolicyDetail: routePath("adminPlatformPolicyDetail"),
  adminPlatformPolicyEdit: routePath("adminPlatformPolicyEdit"),
  adminPlatformPolicyNew: routePath("adminPlatformPolicyNew"),
  dashboard: routePath("dashboard"),
  dashboardAdoption: routePath("dashboardAdoption"),
  dashboardCost: routePath("dashboardCost"),
  dashboardInventory: routePath("dashboardInventory"),
  dashboardProvisioning: routePath("dashboardProvisioning"),
  dashboardReliability: routePath("dashboardReliability"),
  dashboardUsage: routePath("dashboardUsage"),
  gatewayDetail: routePath("gatewayDetail"),
  gatewayList: routePath("gatewayList"),
  gatewayNew: routePath("gatewayNew"),
  home: routes.home,
  login: routePath("login"),
  metrics: routePath("metrics"),
} as const;

export function gatewayDetailPath(gatewayId: string): string {
  return routePaths.gatewayDetail.replace(
    ":gatewayId",
    encodeURIComponent(gatewayId),
  );
}

export function platformPolicyDetailPath(policyId: string): string {
  return routePaths.adminPlatformPolicyDetail.replace(
    ":policyId",
    encodeURIComponent(policyId),
  );
}

export function platformPolicyEditPath(policyId: string): string {
  return routePaths.adminPlatformPolicyEdit.replace(
    ":policyId",
    encodeURIComponent(policyId),
  );
}
