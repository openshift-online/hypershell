import { defineMessages } from "react-intl";

/** Message catalog for the cost dashboard (WEB-COST-08). */
export const costMessages = defineMessages({
  costNavCost: {
    id: "app.dashboard.cost.nav",
    defaultMessage: "Cost",
    description: "Dashboard secondary navigation entry for the cost dashboard.",
  },
  costTitle: {
    id: "app.dashboard.cost.title",
    defaultMessage: "Cost dashboard",
    description: "Main heading on the cost dashboard page.",
  },
  costDescription: {
    id: "app.dashboard.cost.description",
    defaultMessage:
      "Directional operational estimates for capacity planning. This view is not billing-grade reporting.",
    description: "Lead paragraph under the cost dashboard heading.",
  },
  costPurposeHeading: {
    id: "app.dashboard.cost.purposeHeading",
    defaultMessage: "What this dashboard answers",
    description: "Heading for the cost dashboard purpose statement.",
  },
  costPurposePlatform: {
    id: "app.dashboard.cost.purpose.platform",
    defaultMessage:
      "What is the directional operating cost of the platform, where is it concentrated, and how much may be tied to idle resources? It is restricted to system admins and is not billing-grade reporting.",
    description: "Purpose statement for the platform cost view.",
  },
  costPurposeMine: {
    id: "app.dashboard.cost.purpose.mine",
    defaultMessage:
      "What do the gateways I am the billing owner of cost to run, and what do the gateways shared with me cost? Each gateway is charged in full to its billing owner, the owner with the oldest owner grant.",
    description: "Purpose statement for the My cost view.",
  },
  costViewLabel: {
    id: "app.dashboard.cost.view.label",
    defaultMessage: "Cost view",
    description: "Accessible label for the platform / My cost view switch.",
  },
  costViewPlatform: {
    id: "app.dashboard.cost.view.platform",
    defaultMessage: "Platform",
    description: "View switch option for the platform-wide cost view.",
  },
  costViewMine: {
    id: "app.dashboard.cost.view.mine",
    defaultMessage: "My cost",
    description: "View switch option for the signed-in user's cost view.",
  },
  costEstimatesAlertTitle: {
    id: "app.dashboard.cost.estimatesAlert.title",
    defaultMessage: "Estimated costs only",
    description: "Title of the info alert stating values are estimates.",
  },
  costEstimatesAlertBody: {
    id: "app.dashboard.cost.estimatesAlert.body",
    defaultMessage: "Model inputs and limitations are documented.",
    description: "Body of the estimates info alert.",
  },
  costEstimatesLink: {
    id: "app.dashboard.cost.estimatesAlert.link",
    defaultMessage: "About these estimates",
    description: "Link from the estimates alert to the model explanation.",
  },
  costLoading: {
    id: "app.dashboard.cost.loading",
    defaultMessage: "Loading cost dashboard",
    description: "Accessible label for the cost dashboard loading spinner.",
  },
  costLoadErrorTitle: {
    id: "app.dashboard.cost.loadError.title",
    defaultMessage: "Cost metrics are unavailable",
    description: "Title shown when every cost metric source failed.",
  },
  costLoadErrorBody: {
    id: "app.dashboard.cost.loadError.body",
    defaultMessage: "Try refreshing the dashboard in a few minutes.",
    description: "Body shown when every cost metric source failed.",
  },
  costNotConfiguredTitle: {
    id: "app.dashboard.cost.notConfigured.title",
    defaultMessage: "Cost model not configured",
    description:
      "Shown in each cost widget when the deployment has no rate card.",
  },
  costNotConfiguredBody: {
    id: "app.dashboard.cost.notConfigured.body",
    defaultMessage:
      "This deployment has no rate card, so no cost can be estimated.",
    description: "Body for the cost model not configured state.",
  },
  costUnavailable: {
    id: "app.dashboard.cost.unavailable",
    defaultMessage: "Unavailable",
    description: "Shown in a cost widget whose metric source failed.",
  },
  costPartialDataTitle: {
    id: "app.dashboard.cost.partialData.title",
    defaultMessage: "Partial data for the selected range",
    description: "Title of the partial coverage notice.",
  },
  costPartialDataBody: {
    id: "app.dashboard.cost.partialData.body",
    defaultMessage:
      "Metrics are available from {start}. Monthly figures are scaled from the covered period.",
    description: "Body of the partial coverage notice.",
  },
  costUnpricedTitle: {
    id: "app.dashboard.cost.unpriced.title",
    defaultMessage: "Some clusters have no rate",
    description: "Title of the unpriced clusters warning.",
  },
  costUnpricedBody: {
    id: "app.dashboard.cost.unpriced.body",
    defaultMessage: "Excluded from totals: {clusters}.",
    description: "Body of the unpriced clusters warning.",
  },
  costPerMonth: {
    id: "app.dashboard.cost.perMonth",
    defaultMessage: "{amount} / month",
    description: "A monthly cost amount.",
  },
  costWidgetSummary: {
    id: "app.dashboard.cost.widget.summary",
    defaultMessage: "Cost summary",
    description: "Title of the platform cost summary widget.",
  },
  costWidgetIdle: {
    id: "app.dashboard.cost.widget.idle",
    defaultMessage: "Idle resource cost",
    description: "Title of the idle capacity cost widget and summary row.",
  },
  costIdleShare: {
    id: "app.dashboard.cost.idle.share",
    defaultMessage: "{percent}% of estimated monthly cost",
    description: "Share of the total that idle capacity represents.",
  },
  costUnusedGateways: {
    id: "app.dashboard.cost.idle.unusedGateways",
    defaultMessage: "Unused gateways",
    description: "Row for the cost of gateways with no traffic in the range.",
  },
  costWidgetPerUser: {
    id: "app.dashboard.cost.widget.perUser",
    defaultMessage: "Cost per active user",
    description: "Title of the cost per active user widget and summary row.",
  },
  costPerUserCaption: {
    id: "app.dashboard.cost.perUser.caption",
    defaultMessage: "Directional estimate for selected period",
    description: "Caption under the cost per active user value.",
  },
  costPerGateway: {
    id: "app.dashboard.cost.perGateway",
    defaultMessage: "Cost per gateway",
    description: "Summary row for the average monthly cost of a gateway.",
  },
  costPerSandbox: {
    id: "app.dashboard.cost.perSandbox",
    defaultMessage: "Cost per sandbox",
    description: "Summary row for the average monthly cost of a sandbox.",
  },
  costWidgetByEnvironment: {
    id: "app.dashboard.cost.widget.byEnvironment",
    defaultMessage: "By environment",
    description: "Title of the cost by environment widget.",
  },
  costWidgetByCluster: {
    id: "app.dashboard.cost.widget.byCluster",
    defaultMessage: "By cluster",
    description: "Title of the cost by cluster widget.",
  },
  costWidgetByResourceClass: {
    id: "app.dashboard.cost.widget.byResourceClass",
    defaultMessage: "By resource class",
    description: "Title of the cost by resource class widget.",
  },
  costEstimatedMonthly: {
    id: "app.dashboard.cost.estimatedMonthly",
    defaultMessage: "Estimated monthly cost",
    description: "Caption above breakdown rows.",
  },
  costTrendCaption: {
    id: "app.dashboard.cost.trendCaption",
    defaultMessage: "Daily cost, selected period",
    description: "Caption under a daily cost sparkline.",
  },
  costClassGateways: {
    id: "app.dashboard.cost.class.gateways",
    defaultMessage: "Gateways",
    description: "Resource class row for gateway server cost.",
  },
  costClassSandboxes: {
    id: "app.dashboard.cost.class.sandboxes",
    defaultMessage: "Sandboxes",
    description: "Resource class row for sandbox (workspace) cost.",
  },
  costClassPlatform: {
    id: "app.dashboard.cost.class.platform",
    defaultMessage: "Platform overhead",
    description: "Resource class row for non-gateway namespaces.",
  },
  costClassIdle: {
    id: "app.dashboard.cost.class.idle",
    defaultMessage: "Idle capacity",
    description: "Resource class row for unreserved node capacity.",
  },
  costEnvironmentUnknown: {
    id: "app.dashboard.cost.environment.unknown",
    defaultMessage: "Unlabeled",
    description: "Label for clusters without an environment label.",
  },
  costWidgetTopGateways: {
    id: "app.dashboard.cost.widget.topGateways",
    defaultMessage: "Most expensive gateways",
    description: "Title of the top ten gateways by cost widget.",
  },
  costWidgetByBillingOwner: {
    id: "app.dashboard.cost.widget.byBillingOwner",
    defaultMessage: "By billing owner",
    description: "Title of the cost by billing owner widget.",
  },
  costColumnGateway: {
    id: "app.dashboard.cost.column.gateway",
    defaultMessage: "Gateway",
    description: "Table column header for a gateway name.",
  },
  costColumnCluster: {
    id: "app.dashboard.cost.column.cluster",
    defaultMessage: "Cluster",
    description: "Table column header for a cluster.",
  },
  costColumnBillingOwner: {
    id: "app.dashboard.cost.column.billingOwner",
    defaultMessage: "Billing owner",
    description: "Table column header for a gateway's billing owner.",
  },
  costColumnGateways: {
    id: "app.dashboard.cost.column.gateways",
    defaultMessage: "Gateways",
    description: "Table column header for a count of gateways.",
  },
  costColumnMyRole: {
    id: "app.dashboard.cost.column.myRole",
    defaultMessage: "My role",
    description: "Table column header for the signed-in user's role.",
  },
  costColumnWorkspace: {
    id: "app.dashboard.cost.column.workspace",
    defaultMessage: "Workspace",
    description: "Table column header for a workspace.",
  },
  costColumnMonthly: {
    id: "app.dashboard.cost.column.monthly",
    defaultMessage: "Monthly estimate",
    description: "Table column header for a monthly cost estimate.",
  },
  costUnusedLabel: {
    id: "app.dashboard.cost.unusedLabel",
    defaultMessage: "Unused",
    description: "Label on a gateway with no traffic in the range.",
  },
  costChargedToMeLabel: {
    id: "app.dashboard.cost.chargedToMeLabel",
    defaultMessage: "Charged to me",
    description: "Label on a gateway the signed-in user is billing owner of.",
  },
  costRoleOwner: {
    id: "app.dashboard.cost.role.owner",
    defaultMessage: "Owner",
    description: "Gateway role: owner.",
  },
  costRoleAdmin: {
    id: "app.dashboard.cost.role.admin",
    defaultMessage: "Admin",
    description: "Gateway role: admin.",
  },
  costRoleUser: {
    id: "app.dashboard.cost.role.user",
    defaultMessage: "Viewer",
    description: "Gateway role: viewer (user).",
  },
  costWidgetCharged: {
    id: "app.dashboard.cost.widget.charged",
    defaultMessage: "Cost charged to me",
    description: "Title of the My cost charged headline widget.",
  },
  costChargedCaption: {
    id: "app.dashboard.cost.charged.caption",
    defaultMessage:
      "{count, plural, one {# gateway} other {# gateways}} where you are the billing owner",
    description: "Caption under the charged headline.",
  },
  costWidgetShared: {
    id: "app.dashboard.cost.widget.shared",
    defaultMessage: "Shared with me",
    description: "Title of the widget for gateways shared with the user.",
  },
  costSharedCaption: {
    id: "app.dashboard.cost.shared.caption",
    defaultMessage:
      "{count, plural, one {# gateway} other {# gateways}} charged to another billing owner",
    description: "Caption under the shared amount.",
  },
  costWidgetTrend: {
    id: "app.dashboard.cost.widget.trend",
    defaultMessage: "Daily cost charged to me",
    description: "Title of the My cost daily trend widget.",
  },
  costWidgetByGateway: {
    id: "app.dashboard.cost.widget.byGateway",
    defaultMessage: "Cost by gateway",
    description: "Title of the My cost per-gateway table widget.",
  },
  costWidgetByWorkspace: {
    id: "app.dashboard.cost.widget.byWorkspace",
    defaultMessage: "Cost by workspace",
    description: "Title of the My cost per-workspace table widget.",
  },
  costNoRows: {
    id: "app.dashboard.cost.noRows",
    defaultMessage: "No cost recorded in the selected period.",
    description: "Shown in a cost table with no rows.",
  },
  costEmptyTitle: {
    id: "app.dashboard.cost.empty.title",
    defaultMessage: "You have no gateways yet",
    description: "Empty state title for a user with no gateway bindings.",
  },
  costEmptyBody: {
    id: "app.dashboard.cost.empty.body",
    defaultMessage:
      "Cost appears here once you own a gateway or one is shared with you.",
    description: "Empty state body for a user with no gateway bindings.",
  },
  costSystemLabel: {
    id: "app.dashboard.cost.systemLabel",
    defaultMessage: "System",
    description: "Label on platform-view widgets that show system-wide data.",
  },
  costAboutTitle: {
    id: "app.dashboard.cost.about.title",
    defaultMessage: "About these estimates",
    description: "Heading of the cost model explanation section.",
  },
  costAboutModel: {
    id: "app.dashboard.cost.about.model",
    defaultMessage:
      "Cost is estimated from reserved capacity: CPU and memory requests, and node capacity that nothing reserves, multiplied by a rate card supplied by this deployment (version {version}, {currency}). Monthly figures use a 730-hour month.",
    description: "Explains the cost model.",
  },
  costAboutAttribution: {
    id: "app.dashboard.cost.about.attribution",
    defaultMessage:
      "Each gateway, including all its workspaces and sandboxes, is charged to one billing owner: the current owner with the oldest owner grant. If that owner leaves, the gateway's cost in the selected period moves to the next-oldest owner.",
    description: "Explains billing owner attribution.",
  },
  costAboutExcluded: {
    id: "app.dashboard.cost.about.excluded",
    defaultMessage:
      "Not included: storage, network egress, load balancers, managed services such as databases and secrets, and LLM spend.",
    description: "Lists what the cost estimate excludes.",
  },
  costAboutNotBilling: {
    id: "app.dashboard.cost.about.notBilling",
    defaultMessage:
      "These values are directional estimates for capacity planning. They are not billing-grade and are not reconciled with a cloud invoice.",
    description: "States the estimates are not billing-grade.",
  },
});
