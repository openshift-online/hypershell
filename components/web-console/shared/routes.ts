export const routes = {
  dashboard: "dashboard",
  dashboardAdoption: "dashboard/adoption",
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
  dashboard: routePath("dashboard"),
  dashboardAdoption: routePath("dashboardAdoption"),
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
