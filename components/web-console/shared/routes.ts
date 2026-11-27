export const routes = {
  adminPlatformPolicies: "admin/platform-policies",
  adminPlatformPolicyDetail: "admin/platform-policies/:policyId",
  adminPlatformPolicyEdit: "admin/platform-policies/:policyId/edit",
  adminPlatformPolicyNew: "admin/platform-policies/new",
  dashboard: "dashboard",
  dashboardAdoption: "dashboard/adoption",
  dashboardInventory: "dashboard/inventory",
  dashboardProvisioning: "dashboard/provisioning",
  dashboardReliability: "dashboard/reliability",
  dashboardUsage: "dashboard/usage",
  docs: "docs",
  docsCliInstallHsctl: "docs/cli/install-hsctl",
  docsRefusalResourceLimits: "docs/refusals/resource-limits",
  docsRunbookGithubJira: "docs/runbooks/github-jira",
  docsRunbookGitlabJira: "docs/runbooks/gitlab-jira",
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
  dashboardInventory: routePath("dashboardInventory"),
  dashboardProvisioning: routePath("dashboardProvisioning"),
  dashboardReliability: routePath("dashboardReliability"),
  dashboardUsage: routePath("dashboardUsage"),
  docs: routePath("docs"),
  docsCliInstallHsctl: routePath("docsCliInstallHsctl"),
  docsRefusalResourceLimits: routePath("docsRefusalResourceLimits"),
  docsRunbookGithubJira: routePath("docsRunbookGithubJira"),
  docsRunbookGitlabJira: routePath("docsRunbookGitlabJira"),
  gatewayDetail: routePath("gatewayDetail"),
  gatewayList: routePath("gatewayList"),
  gatewayNew: routePath("gatewayNew"),
  home: routes.home,
  login: routePath("login"),
  metrics: routePath("metrics"),
} as const;

/**
 * The public docs routes (WEB-UDOC-01). The BFF serves the SPA document for
 * exactly these paths without a session (WEB-UDOC-02).
 */
export const docsRouteNames = [
  "docs",
  "docsCliInstallHsctl",
  "docsRefusalResourceLimits",
  "docsRunbookGithubJira",
  "docsRunbookGitlabJira",
] as const satisfies readonly RouteName[];

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
