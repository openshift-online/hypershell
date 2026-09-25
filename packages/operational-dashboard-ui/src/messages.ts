import { defineMessages } from "react-intl";

export const messages = defineMessages({
  addWidgets: {
    id: "app.dashboard.addWidgets",
    defaultMessage: "Add widgets",
    description: "Label for the button that opens the widget drawer.",
  },
  apiErrorRateWidget: {
    id: "app.dashboard.widget.apiErrorRate",
    defaultMessage: "API error rate",
    description: "Title for the API 5xx error rate reliability widget.",
  },
  apiLatencyWidget: {
    id: "app.dashboard.widget.apiLatency",
    defaultMessage: "API latency",
    description: "Title for the API median latency reliability widget.",
  },
  apiRequestRateWidget: {
    id: "app.dashboard.widget.apiRequestRate",
    defaultMessage: "API request rate",
    description: "Title for the API request rate reliability widget.",
  },
  cpus: {
    id: "app.dashboard.summary.cpus",
    defaultMessage: "CPUs",
    description: "Summary label for provisioned CPU capacity.",
  },
  dashboardHelpApiErrorRate: {
    id: "app.dashboard.help.apiErrorRate",
    defaultMessage:
      "The current API error rate is the percentage of API requests that returned an HTTP 5xx response over the last 5 minutes. Each trend point is an hourly sample from the previous 24 hours. Its value is the 5xx error rate calculated from the 5 minutes immediately before that sample's timestamp.\nHTTP 4xx responses, such as 401 or 403 responses, aren't included in the error rate.",
    description: "Help text for the API error rate widget.",
  },
  dashboardHelpApiLatency: {
    id: "app.dashboard.help.apiLatency",
    defaultMessage:
      "The current API latency is the median response time for API requests over the last 5 minutes. Each trend point is an hourly sample from the previous 24 hours. Its value is the median latency calculated from the 5 minutes immediately before that sample's timestamp.\nLatency is shown in seconds. The median means that half of the requests were completed in less time and half took longer.",
    description: "Help text for the API latency widget.",
  },
  dashboardHelpApiRequestRate: {
    id: "app.dashboard.help.apiRequestRate",
    defaultMessage:
      "The current API request rate is the average number of requests per second over the last 5 minutes. Each trend point is an hourly sample from the previous 24 hours. Its value is calculated from the 5 minutes immediately before that sample's timestamp.\nThe API request rate includes inbound requests to the HyperShell API endpoints.",
    description: "Help text for the API request rate widget.",
  },
  dashboardHelpReconciliationFailures: {
    id: "app.dashboard.help.reconciliationFailures",
    defaultMessage:
      "The current value is the number of failed reconciliation attempts during the last 24 hours. Each trend point shows the number of failures recorded during a one-hour period across the previous 24 hours.",
    description: "Help text for the reconciliation failures widget.",
  },
  dashboardHelpReconciliationLag: {
    id: "app.dashboard.help.reconciliationLag",
    defaultMessage:
      "The current reconciliation lag is the median time, in seconds, from when a reconciliation becomes eligible to when it completes, measured over the last 5 minutes. Each trend point is an hourly sample from the previous 24 hours. Its value is the median lag calculated from the observations available at that sample's timestamp.",
    description: "Help text for the reconciliation lag widget.",
  },
  dashboardHelpReconciliationRetries: {
    id: "app.dashboard.help.reconciliationRetries",
    defaultMessage:
      "The current value is the number of reconciliation retries during the last 24 hours. Each trend point shows the number of retries recorded during a one-hour period across the previous 24 hours.",
    description: "Help text for the reconciliation retries widget.",
  },
  dashboardHelpUsers: {
    id: "app.dashboard.help.users",
    defaultMessage:
      "The current Users value is the total number of registered users in HyperShell. It isn't a count of users who are online right now.\nThe trend shows the number of distinct registered users who used the API on each UTC calendar day during the last 30 days. A user is counted once per day even if they made multiple API requests.",
    description: "Help text for the Users widget.",
  },
  description: {
    id: "app.dashboard.description",
    defaultMessage:
      "Live view of gateway fleet health, hub cluster capacity, and platform adoption. Metrics refresh every 15 minutes.",
    description: "Supporting text on the operational dashboard page.",
  },
  gatewayReleasesSummaryAriaLabel: {
    id: "app.dashboard.gatewayReleases.summaryAriaLabel",
    defaultMessage: "Gateways by release",
    description:
      "Accessible label for the gateway releases stat panel in the dashboard.",
  },
  gatewayReleasesWidget: {
    id: "app.dashboard.widget.gatewayReleases",
    defaultMessage: "Gateway releases",
    description: "Title for the gateway releases dashboard widget.",
  },
  gateways: {
    id: "app.dashboard.summary.gateways",
    defaultMessage: "Gateways",
    description: "Summary label for provisioned gateways.",
  },
  gatewayStatusAriaDesc: {
    id: "app.dashboard.gatewayStatus.ariaDesc",
    defaultMessage: "Gateway count by status",
    description: "Accessible description for the gateway status donut chart.",
  },
  gatewayStatusChartTitle: {
    id: "app.dashboard.gatewayStatus.chartTitle",
    defaultMessage: "Gateway status chart",
    description: "Accessible title for the gateway status donut chart.",
  },
  gatewayStatusDataLabel: {
    id: "app.dashboard.gatewayStatus.dataLabel",
    defaultMessage: "{status}: {count}",
    description:
      "Data label for a gateway status donut chart segment. Superseded by statusDonutDataLabel.",
  },
  gatewayStatusDegraded: {
    id: "app.dashboard.gatewayStatus.degraded",
    defaultMessage: "Degraded",
    description: "Legend label for degraded gateways.",
  },
  gatewayStatusFailed: {
    id: "app.dashboard.gatewayStatus.failed",
    defaultMessage: "Failed",
    description: "Legend label for failed gateways.",
  },
  gatewayStatusHealthy: {
    id: "app.dashboard.gatewayStatus.healthy",
    defaultMessage: "Healthy",
    description: "Legend label for healthy gateways.",
  },
  gatewayStatusLegend: {
    id: "app.dashboard.gatewayStatus.legend",
    defaultMessage: "{status}: {count}",
    description: "Legend entry for a gateway status donut chart segment.",
  },
  gatewayStatusProvisioning: {
    id: "app.dashboard.gatewayStatus.provisioning",
    defaultMessage: "Provisioning",
    description: "Legend label for provisioning gateways.",
  },
  gatewayStatusWidget: {
    id: "app.dashboard.widget.gatewayStatus",
    defaultMessage: "Gateways",
    description: "Title for the gateways dashboard widget.",
  },
  inventoryStatusUnknown: {
    id: "app.dashboard.inventoryStatus.unknown",
    defaultMessage: "Unknown",
    description: "Display label for inventory status bucket key unknown.",
  },
  inventoryStatusWarning: {
    id: "app.dashboard.inventoryStatus.warning",
    defaultMessage: "Warning",
    description:
      "Summary label for inventory items in a non-failed exception state (for example degraded or pending).",
  },
  inventorySummaryAriaLabel: {
    id: "app.dashboard.summary.inventoryAriaLabel",
    defaultMessage: "Platform inventory metrics",
    description: "Accessible label for the platform inventory summary widget.",
  },
  inventorySummaryWidget: {
    id: "app.dashboard.widget.inventorySummary",
    defaultMessage: "Inventory summary",
    description: "Title for the platform inventory summary dashboard widget.",
  },
  lastRefreshed: {
    id: "app.dashboard.lastRefreshed",
    defaultMessage: "Last refreshed: {timestamp}",
    description:
      "Timestamp shown when operational dashboard metrics were last loaded successfully.",
  },
  loadErrorBody: {
    id: "app.dashboard.loadError.body",
    defaultMessage:
      "An unexpected error occurred while loading dashboard metrics.",
    description:
      "Recovery guidance when operational dashboard metrics cannot be loaded.",
  },
  loadErrorTitle: {
    id: "app.dashboard.loadError.title",
    defaultMessage: "Operational dashboard metrics are unavailable",
    description:
      "Title shown when operational dashboard metrics cannot be loaded.",
  },
  loading: {
    id: "app.dashboard.loading",
    defaultMessage: "Loading operational dashboard metrics",
    description:
      "Accessible status shown while operational dashboard metrics load.",
  },
  managedClusterProvidersAriaDesc: {
    id: "app.dashboard.managedClusterProviders.ariaDesc",
    defaultMessage: "Managed cluster count by provider",
    description:
      "Accessible description for the managed cluster provider donut chart.",
  },
  managedClusterProvidersChartTitle: {
    id: "app.dashboard.managedClusterProviders.chartTitle",
    defaultMessage: "Managed cluster providers chart",
    description:
      "Accessible title for the managed cluster provider donut chart.",
  },
  managedClusterRegionsAriaDesc: {
    id: "app.dashboard.managedClusterRegions.ariaDesc",
    defaultMessage: "Managed cluster count by region and provider",
    description:
      "Accessible description for the managed cluster placement donut chart.",
  },
  managedClusterRegionsChartTitle: {
    id: "app.dashboard.managedClusterRegions.chartTitle",
    defaultMessage: "Managed cluster regions chart",
    description: "Accessible title for the managed cluster region donut chart.",
  },
  managedClustersCreatedLast30Days: {
    id: "app.dashboard.summary.managedClustersCreatedLast30Days",
    defaultMessage: "Clusters created (30 days)",
    description:
      "Summary label for managed clusters created within the lookback window.",
  },
  managedClustersSummary: {
    id: "app.dashboard.summary.managedClusters",
    defaultMessage: "Clusters",
    description: "Summary label for total managed clusters.",
  },
  memory: {
    id: "app.dashboard.summary.memory",
    defaultMessage: "Memory",
    description: "Summary label for memory utilization.",
  },
  metricCouldNotBeDetermined: {
    id: "app.dashboard.metricCouldNotBeDetermined",
    defaultMessage: "Metric could not be determined",
    description:
      "Fallback when a dashboard metric value is non-finite or cannot be shown as a number.",
  },
  metricSourceGatewayReleaseDistribution: {
    id: "app.dashboard.metricSource.gatewayReleaseDistribution",
    defaultMessage: "Gateway releases",
    description:
      "Label for the gateway release distribution metric source in partial-load warnings.",
  },
  metricUnavailableBody: {
    id: "app.dashboard.metricUnavailable.body",
    defaultMessage: "This information is not currently available.",
    description:
      "Recovery guidance when an individual dashboard metric is missing.",
  },
  metricUnavailableTitle: {
    id: "app.dashboard.metricUnavailable.title",
    defaultMessage: "Metric unavailable",
    description:
      "Heading shown when an individual dashboard metric is missing.",
  },
  metricValue: {
    id: "app.dashboard.metric.value",
    defaultMessage: "{value} {label}",
    description: "Formatted count for a dashboard metric card heading.",
  },
  nodes: {
    id: "app.dashboard.widget.nodes",
    defaultMessage: "Nodes",
    description: "Title for the nodes dashboard widget.",
  },
  nodeStatusAriaDesc: {
    id: "app.dashboard.nodeStatus.ariaDesc",
    defaultMessage: "Node count by readiness",
    description: "Accessible description for the node status donut chart.",
  },
  nodeStatusChartTitle: {
    id: "app.dashboard.nodeStatus.chartTitle",
    defaultMessage: "Node status chart",
    description: "Accessible title for the node status donut chart.",
  },
  nodeStatusNotReady: {
    id: "app.dashboard.nodeStatus.notReady",
    defaultMessage: "Not ready",
    description: "Legend label for not-ready nodes.",
  },
  nodeStatusReady: {
    id: "app.dashboard.nodeStatus.ready",
    defaultMessage: "Ready",
    description: "Legend label for ready nodes.",
  },
  partialLoadWarningBody: {
    id: "app.dashboard.partialLoadWarning.body",
    defaultMessage:
      "Some dashboard metrics could not be loaded. Available data is shown below; missing areas may be incomplete.",
    description:
      "Recovery guidance when one or more operational dashboard metric sources fail.",
  },
  partialLoadWarningRefreshAction: {
    id: "app.dashboard.partialLoadWarning.refreshAction",
    defaultMessage: "Refresh all data now",
    description:
      "Action link on the partial-load warning that retries every dashboard metric source.",
  },
  partialLoadWarningTitle: {
    id: "app.dashboard.partialLoadWarning.title",
    defaultMessage: "Some dashboard metrics are unavailable",
    description:
      "Title shown when one or more operational dashboard metric sources fail.",
  },
  pods: {
    id: "app.dashboard.summary.pods",
    defaultMessage: "Pods",
    description: "Summary label for pod utilization.",
  },
  podStatusAriaDesc: {
    id: "app.dashboard.podStatus.ariaDesc",
    defaultMessage: "Pod capacity by phase and unused slots",
    description: "Accessible description for the pod capacity donut chart.",
  },
  podStatusChartTitle: {
    id: "app.dashboard.podStatus.chartTitle",
    defaultMessage: "Pod capacity chart",
    description: "Accessible title for the pod capacity donut chart.",
  },
  podStatusFailed: {
    id: "app.dashboard.podStatus.failed",
    defaultMessage: "Failed",
    description: "Legend label for failed pods.",
  },
  podStatusPending: {
    id: "app.dashboard.podStatus.pending",
    defaultMessage: "Pending",
    description: "Legend label for pending pods.",
  },
  podStatusRunning: {
    id: "app.dashboard.podStatus.running",
    defaultMessage: "Running",
    description: "Legend label for running pods.",
  },
  podStatusSucceeded: {
    id: "app.dashboard.podStatus.succeeded",
    defaultMessage: "Succeeded",
    description: "Legend label for succeeded pods.",
  },
  podStatusUnknown: {
    id: "app.dashboard.podStatus.unknown",
    defaultMessage: "Unknown",
    description: "Legend label for unknown-phase pods.",
  },
  podStatusUnused: {
    id: "app.dashboard.podStatus.unused",
    defaultMessage: "Unused",
    description: "Legend label for unused pod capacity slots.",
  },
  provisionedGateways: {
    id: "app.dashboard.widget.provisionedGateways",
    defaultMessage: "Provisioned gateways",
    description: "Title for the provisioned gateways dashboard widget.",
  },
  provisionReliabilityAriaDesc: {
    id: "app.dashboard.provisionReliability.ariaDesc",
    defaultMessage: "Gateway provision success and failure counts",
    description:
      "Accessible description for the provision reliability donut chart.",
  },
  provisionReliabilityChartTitle: {
    id: "app.dashboard.provisionReliability.chartTitle",
    defaultMessage: "Gateway provision reliability chart",
    description: "Accessible title for the provision reliability donut chart.",
  },
  provisionReliabilityFailures: {
    id: "app.dashboard.provisionReliability.failures",
    defaultMessage: "Failures",
    description: "Legend label for failed gateway provisions.",
  },
  provisionReliabilityHourlySuccessRate: {
    id: "app.dashboard.provisionReliability.hourlySuccessRate",
    defaultMessage: "Hourly success rate",
    description:
      "Title for the hourly provision success-rate sparkline in the provision reliability widget.",
  },
  provisionReliabilityLast24Hours: {
    id: "app.dashboard.provisionReliability.last24Hours",
    defaultMessage: "Last 24 hours",
    description:
      "Caption documenting the rolling 24-hour provision reliability lookback window.",
  },
  provisionReliabilityLifetimeSuccessCount: {
    id: "app.dashboard.provisionReliability.lifetimeSuccessCount",
    defaultMessage: "Lifetime success count",
    description:
      "Caption when provision reliability success counts come from the lifetime duration histogram fallback.",
  },
  provisionReliabilityRate: {
    id: "app.dashboard.provisionReliability.rate",
    defaultMessage: "{rate}%",
    description:
      "Center title for the provision reliability donut showing the 24-hour success rate.",
  },
  provisionReliabilitySuccesses: {
    id: "app.dashboard.provisionReliability.successes",
    defaultMessage: "Successes",
    description: "Legend label for successful gateway provisions.",
  },
  provisionReliabilityWidget: {
    id: "app.dashboard.widget.provisionReliability",
    defaultMessage: "Gateway provision reliability",
    description:
      "Title for the gateway provision reliability dashboard widget.",
  },
  provisionSuccessRate24h: {
    id: "app.dashboard.summary.provisionSuccessRate24h",
    defaultMessage: "Success rate (24h)",
    description:
      "Summary label for the rolling 24-hour gateway provision success rate.",
  },
  provisionSuccessRateLifetime: {
    id: "app.dashboard.summary.provisionSuccessRateLifetime",
    defaultMessage: "Success rate (lifetime)",
    description:
      "Summary label when provision reliability uses the lifetime duration histogram fallback.",
  },
  provisionTime: {
    id: "app.dashboard.summary.provisionTime",
    defaultMessage: "Provision time (average)",
    description: "Summary label for average gateway provision time.",
  },
  provisionTimeAverage: {
    id: "app.dashboard.provisionTime.average",
    defaultMessage: "Average",
    description: "Label for average gateway provision duration.",
  },
  provisionTimeMedian: {
    id: "app.dashboard.provisionTime.median",
    defaultMessage: "Median (P50)",
    description: "Label for median gateway provision duration.",
  },
  provisionTimeP50: {
    id: "app.dashboard.summary.provisionTimeP50",
    defaultMessage: "Provision time (P50)",
    description: "Deprecated summary label retained for locale extraction.",
  },
  provisionTimeP95: {
    id: "app.dashboard.summary.provisionTimeP95",
    defaultMessage: "Provision time (P95)",
    description: "Deprecated summary label retained for locale extraction.",
  },
  provisionTimeP95Label: {
    id: "app.dashboard.provisionTime.p95",
    defaultMessage: "P95",
    description: "Label for the 95th percentile gateway provision duration.",
  },
  provisionTimeP95Note: {
    id: "app.dashboard.provisionTime.p95Note",
    defaultMessage:
      "95% of gateways were provisioned in under {duration} {unit}.",
    description:
      "Context note explaining the 95th percentile gateway provision duration.",
  },
  provisionTimeStatsAriaLabel: {
    id: "app.dashboard.provisionTime.statsAriaLabel",
    defaultMessage: "Gateway provision duration statistics",
    description:
      "Accessible label for the provision time statistics description list.",
  },
  provisionTimeWidget: {
    id: "app.dashboard.widget.provisionTime",
    defaultMessage: "Gateway provision time",
    description: "Title for the gateway provision time dashboard widget.",
  },
  refresh: {
    id: "app.dashboard.refresh",
    defaultMessage: "Refresh dashboard metrics",
    description:
      "Accessible label for refreshing operational dashboard metrics.",
  },
  refreshErrorBody: {
    id: "app.dashboard.refreshError.body",
    defaultMessage:
      "Showing the last successful metrics. Try refreshing again.",
    description:
      "Recovery guidance when operational dashboard metrics cannot be refreshed.",
  },
  refreshErrorTitle: {
    id: "app.dashboard.refreshError.title",
    defaultMessage: "Could not refresh dashboard metrics",
    description:
      "Title shown when operational dashboard metrics cannot be refreshed.",
  },
  registeredUsers: {
    id: "app.dashboard.widget.registeredUsers",
    defaultMessage: "Users",
    description: "Title for the registered users dashboard widget.",
  },
  registeredUsersAdded7Days: {
    id: "app.dashboard.registeredUsers.added7Days",
    defaultMessage: "Added (7 days)",
    description: "Stat label for users added in the last 7 days.",
  },
  registeredUsersAdded30Days: {
    id: "app.dashboard.registeredUsers.added30Days",
    defaultMessage: "Added (30 days)",
    description: "Stat label for users added in the last 30 days.",
  },
  registeredUsersHero: {
    id: "app.dashboard.registeredUsers.hero",
    defaultMessage: "{value} Registered users",
    description: "Hero line for the Users adoption widget.",
  },
  registeredUsersSummary: {
    id: "app.dashboard.summary.registeredUsers",
    defaultMessage: "Users",
    description: "Summary label for registered users.",
  },
  registeredUsersUniqueLogins: {
    id: "app.dashboard.registeredUsers.uniqueLogins",
    defaultMessage: "Unique logins",
    description: "Metric name in registered users login sparkline tooltips.",
  },
  registeredUsersUniqueLogins7Days: {
    id: "app.dashboard.registeredUsers.uniqueLogins7Days",
    defaultMessage: "Unique logins (7 days)",
    description: "Stat label for unique logins in the last 7 days.",
  },
  registeredUsersUniqueLogins30Days: {
    id: "app.dashboard.registeredUsers.uniqueLogins30Days",
    defaultMessage: "Unique logins (30 days)",
    description: "Stat label for unique logins in the last 30 days.",
  },
  registeredUsersUniqueLoginsPerDay: {
    id: "app.dashboard.registeredUsers.uniqueLoginsPerDay",
    defaultMessage: "Unique logins per day",
    description: "Title above the registered users login sparkline.",
  },
  reliabilityDescription: {
    id: "app.dashboard.reliability.description",
    defaultMessage:
      "API request rate, 5xx error rate, and median latency with 24-hour hourly trends. Metrics refresh every 15 minutes.",
    description: "Supporting text on the reliability dashboard page.",
  },
  reliabilityLoadErrorBody: {
    id: "app.dashboard.reliability.loadError.body",
    defaultMessage:
      "An unexpected error occurred while loading reliability metrics.",
    description:
      "Recovery guidance when reliability dashboard metrics cannot be loaded.",
  },
  reliabilityLoadErrorTitle: {
    id: "app.dashboard.reliability.loadError.title",
    defaultMessage: "Reliability dashboard metrics are unavailable",
    description:
      "Title shown when reliability dashboard metrics cannot be loaded.",
  },
  reliabilityLoading: {
    id: "app.dashboard.reliability.loading",
    defaultMessage: "Loading reliability dashboard metrics",
    description:
      "Accessible status shown while reliability dashboard metrics load.",
  },
  reliabilityNavAriaLabel: {
    id: "app.dashboard.nav.ariaLabel",
    defaultMessage: "Dashboard views",
    description:
      "Accessible name for the secondary navigation between operational and reliability dashboards.",
  },
  reliabilityNavOperational: {
    id: "app.dashboard.nav.operational",
    defaultMessage: "Operational",
    description:
      "Secondary navigation label for the operational dashboard route.",
  },
  reliabilityNavReliability: {
    id: "app.dashboard.nav.reliability",
    defaultMessage: "Reliability",
    description:
      "Secondary navigation label for the reliability dashboard route.",
  },
  reliabilitySummaryAriaLabel: {
    id: "app.dashboard.reliability.summaryAriaLabel",
    defaultMessage: "API reliability summary",
    description: "Accessible label for the reliability summary widget.",
  },
  reliabilitySummaryErrorRate: {
    id: "app.dashboard.reliability.summary.errorRate",
    defaultMessage: "Error rate",
    description: "Summary row label for API 5xx error rate.",
  },
  reliabilitySummaryLatency: {
    id: "app.dashboard.reliability.summary.latency",
    defaultMessage: "Median latency",
    description: "Summary row label for API median latency.",
  },
  reliabilitySummaryRequestRate: {
    id: "app.dashboard.reliability.summary.requestRate",
    defaultMessage: "Request rate",
    description: "Summary row label for API request rate.",
  },
  reliabilitySummaryWidget: {
    id: "app.dashboard.widget.reliabilitySummary",
    defaultMessage: "Reliability summary",
    description: "Title for the reliability summary dashboard widget.",
  },
  reliabilityTitle: {
    id: "app.dashboard.reliability.title",
    defaultMessage: "Reliability",
    description: "Main heading on the reliability dashboard page.",
  },
  resetToDefault: {
    id: "app.dashboard.resetToDefault",
    defaultMessage: "Reset to default",
    description:
      "Label for restoring the operational dashboard default layout.",
  },
  sandboxActiveSandboxesPerDay: {
    id: "app.dashboard.sandboxStatus.activeSandboxesPerDay",
    defaultMessage: "Active sandboxes per day",
    description: "Title above the daily active-sandboxes sparkline.",
  },
  sandboxActiveSandboxesPerHour: {
    id: "app.dashboard.sandboxStatus.activeSandboxesPerHour",
    defaultMessage: "Active sandboxes per hour",
    description: "Title above the hourly active-sandboxes sparkline.",
  },
  sandboxAttentionExpiringSoon: {
    id: "app.dashboard.sandboxStatus.expiringSoon",
    defaultMessage: "{count} Expiring soon",
    description: "Attention label for sandboxes expiring within 24 hours.",
  },
  sandboxAttentionIdle: {
    id: "app.dashboard.sandboxStatus.idle",
    defaultMessage: "{count} Idle",
    description: "Attention label for idle sandboxes with a live Gateway.",
  },
  sandboxAttentionOrphaned: {
    id: "app.dashboard.sandboxStatus.orphaned",
    defaultMessage: "{count} Orphaned",
    description: "Attention label for sandboxes whose owning Gateway is gone.",
  },
  sandboxAttentionRequired: {
    id: "app.dashboard.sandboxStatus.attentionRequired",
    defaultMessage: "Attention required",
    description:
      "Heading for the sandbox status attention labels (expiring, orphaned, idle).",
  },
  sandboxAttentionUnavailable: {
    id: "app.dashboard.sandboxStatus.attentionUnavailable",
    defaultMessage: "Attention counts unavailable",
    description:
      "Shown when Active loads but orphaned/expiring/idle attention fields are absent.",
  },
  sandboxStatusActive: {
    id: "app.dashboard.sandboxStatus.active",
    defaultMessage: "Active",
    description:
      "Legend label for active sandboxes when no status breakdown exists.",
  },
  sandboxStatusAriaDesc: {
    id: "app.dashboard.sandboxStatus.ariaDesc",
    defaultMessage: "Active sandboxes across the gateway fleet.",
    description: "Accessible description for the sandbox status donut chart.",
  },
  sandboxStatusChartTitle: {
    id: "app.dashboard.sandboxStatus.chartTitle",
    defaultMessage: "Sandbox status",
    description: "Accessible title for the sandbox status donut chart.",
  },
  sandboxStatusWidget: {
    id: "app.dashboard.widget.sandboxStatus",
    defaultMessage: "Sandbox status",
    description: "Title for the sandbox status dashboard widget.",
  },
  sectionTitleDefault: {
    id: "app.dashboard.widget.sectionTitle",
    defaultMessage: "Section title",
    description:
      "Default label for a section title widget in the add-widgets drawer.",
  },
  sectionTitleHubCluster: {
    id: "app.dashboard.sectionTitle.hubCluster",
    defaultMessage: "Hub cluster",
    description: "Section title above hub cluster capacity widgets.",
  },
  sectionTitlePlatformAdoption: {
    id: "app.dashboard.sectionTitle.platformAdoption",
    defaultMessage: "Platform adoption",
    description: "Section title above platform adoption widgets.",
  },
  sectionTitlePlatformInventory: {
    id: "app.dashboard.sectionTitle.platformInventory",
    defaultMessage: "Platform inventory",
    description: "Section title above platform inventory widgets.",
  },
  statusDonutDataLabel: {
    id: "app.dashboard.statusDonut.dataLabel",
    defaultMessage: "{status}: {count}",
    description: "Data label for a status donut chart segment.",
  },
  statusDonutLegend: {
    id: "app.dashboard.statusDonut.legend",
    defaultMessage: "{status}: {count}",
    description: "Legend entry for a status donut chart segment.",
  },
  summary: {
    id: "app.dashboard.widget.summary",
    defaultMessage: "Summary",
    description: "Title for the operational dashboard summary widget.",
  },
  summarySystem: {
    id: "app.dashboard.summary.system",
    defaultMessage: "System",
    description:
      "Heading for system utilization metrics in the summary widget.",
  },
  summarySystemAriaLabel: {
    id: "app.dashboard.summary.systemAriaLabel",
    defaultMessage: "System metrics",
    description:
      "Accessible label for the system metrics list in the summary widget.",
  },
  summaryTrendDecrease: {
    id: "app.dashboard.summary.trendDecrease",
    defaultMessage: "{percent}% decrease in {subject}",
    description:
      "Tooltip for a usage summary metric that decreased since the start of its trend.",
  },
  summaryTrendIncrease: {
    id: "app.dashboard.summary.trendIncrease",
    defaultMessage: "{percent}% increase in {subject}",
    description:
      "Tooltip for a usage summary metric that increased since the start of its trend.",
  },
  summaryUsage: {
    id: "app.dashboard.summary.usage",
    defaultMessage: "Usage",
    description: "Heading for adoption metrics in the summary widget.",
  },
  summaryUsageAriaLabel: {
    id: "app.dashboard.summary.usageAriaLabel",
    defaultMessage: "Usage metrics",
    description:
      "Accessible label for the usage metrics list in the summary widget.",
  },
  systemSummaryWidget: {
    id: "app.dashboard.widget.systemSummary",
    defaultMessage: "System summary",
    description: "Title for the operational dashboard system summary widget.",
  },
  title: {
    id: "app.dashboard.title",
    defaultMessage: "Operational",
    description: "Main heading on the operational dashboard page.",
  },
  trendLast7Days: {
    id: "app.dashboard.trend.last7Days",
    defaultMessage: "Last 7 days",
    description:
      "Caption below a seven-day trend sparkline on the operational dashboard.",
  },
  trendLast24Hours: {
    id: "app.dashboard.trend.last24Hours",
    defaultMessage: "Last 24 hours",
    description:
      "Caption below a twenty-four-hour trend sparkline on the operational dashboard.",
  },
  trendLastDays: {
    id: "app.dashboard.trend.lastDays",
    defaultMessage: "Last {days} days",
    description: "Caption below a trend sparkline showing the lookback window.",
  },
  trendTooltip: {
    id: "app.dashboard.trend.tooltip",
    defaultMessage: "{date}: {value} {metric}",
    description: "Tooltip for a dashboard metric trend sparkline point.",
  },
  usageSummaryWidget: {
    id: "app.dashboard.widget.usageSummary",
    defaultMessage: "Usage summary",
    description: "Title for the operational dashboard usage summary widget.",
  },
  utilizationCapacity: {
    id: "app.dashboard.utilization.capacity",
    defaultMessage: "{unit} capacity",
    description: "Capacity label for a utilization donut chart.",
  },
  utilizationChartTitle: {
    id: "app.dashboard.utilization.chartTitle",
    defaultMessage: "{unit} utilization chart",
    description: "Accessible title for a utilization donut chart.",
  },
  utilizationDataLabel: {
    id: "app.dashboard.utilization.dataLabel",
    defaultMessage: "{capacity}: {percentage}%",
    description: "Data label for a utilization donut chart segment.",
  },
  utilizationLabel: {
    id: "app.dashboard.utilization.label",
    defaultMessage: "{value} {unit}",
    description: "Primary value label for a utilization donut chart.",
  },
  utilizationSubtitle: {
    id: "app.dashboard.utilization.subtitle",
    defaultMessage: "of {total} {unit}",
    description: "Subtitle for a utilization donut chart.",
  },
  utilizationSummaryTooltip: {
    id: "app.dashboard.utilization.summaryTooltip",
    defaultMessage: "{percent}% capacity{separator}{value} of {total} {unit}",
    description: "Tooltip for utilization status in the system summary widget.",
  },
  widgetCpu: {
    id: "app.dashboard.widget.cpu",
    defaultMessage: "CPU",
    description: "Title for the CPU utilization dashboard widget.",
  },
  widgetManagedClusterProviders: {
    id: "app.dashboard.widget.managedClusterProviders",
    defaultMessage: "Cluster providers",
    description:
      "Title for the managed cluster provider breakdown dashboard widget.",
  },
  widgetManagedClusterRegions: {
    id: "app.dashboard.widget.managedClusterRegions",
    defaultMessage: "Cluster regions",
    description:
      "Title for the managed cluster region breakdown dashboard widget.",
  },
  widgetMemory: {
    id: "app.dashboard.widget.memory",
    defaultMessage: "Memory",
    description: "Title for the memory utilization dashboard widget.",
  },
  widgetPods: {
    id: "app.dashboard.widget.pods",
    defaultMessage: "Pods",
    description: "Title for the pods utilization dashboard widget.",
  },
  widgetSandboxes: {
    id: "app.dashboard.summary.sandboxes",
    defaultMessage: "Sandboxes",
    description:
      "Label for provisioned sandboxes on the operational dashboard.",
  },
});
