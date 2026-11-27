import {
  index,
  layout,
  route,
  type RouteConfig,
} from "@react-router/dev/routes";

import { routes } from "../shared/routes";

export default [
  route(routes.login, "./routes/login.tsx"),
  layout("./routes/application.tsx", [
    index("./routes/home.tsx"),
    route(routes.dashboard, "./routes/dashboard.tsx"),
    route(routes.dashboardProvisioning, "./routes/dashboard-provisioning.tsx"),
    route(routes.dashboardInventory, "./routes/dashboard-inventory.tsx"),
    route(routes.dashboardReliability, "./routes/dashboard-reliability.tsx"),
    route(routes.dashboardUsage, "./routes/dashboard-usage.tsx"),
    route(routes.dashboardAdoption, "./routes/dashboard-adoption.tsx"),
    route(routes.gatewayList, "./routes/gateways.tsx"),
    route(routes.gatewayNew, "./routes/gateway-new.tsx"),
    route(routes.gatewayDetail, "./routes/gateway.tsx"),
    route(routes.metrics, "./routes/metrics.tsx"),
    // Public docs routes (WEB-UDOC-01). Each is its own lazily loaded route
    // module, so docs code adds nothing to other route bundles.
    route(routes.docs, "./routes/docs.tsx"),
    route(
      routes.docsRunbookGithubJira,
      "./routes/docs-runbook-github-jira.tsx",
    ),
    route(
      routes.docsRunbookGitlabJira,
      "./routes/docs-runbook-gitlab-jira.tsx",
    ),
    route(routes.docsCliInstallHsctl, "./routes/docs-cli-install-hsctl.tsx"),
    route(
      routes.docsRefusalResourceLimits,
      "./routes/docs-refusal-resource-limits.tsx",
    ),
    route(routes.adminPlatformPolicies, "./routes/admin-platform-policies.tsx"),
    route(
      routes.adminPlatformPolicyNew,
      "./routes/admin-platform-policy-new.tsx",
    ),
    route(
      routes.adminPlatformPolicyDetail,
      "./routes/admin-platform-policy.tsx",
    ),
    route(
      routes.adminPlatformPolicyEdit,
      "./routes/admin-platform-policy-edit.tsx",
    ),
  ]),
  route("*", "./routes/not-found.tsx"),
] satisfies RouteConfig;
