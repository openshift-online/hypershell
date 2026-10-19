import { defineMessages, type MessageDescriptor } from "react-intl";

import type { ReliabilityMetricId } from "./application/dashboard-types";

export const messages = defineMessages({
  activeSandboxesWidget: {
    id: "app.dashboard.widget.activeSandboxes",
    defaultMessage: "Active sandboxes",
    description: "Title for the active sandboxes dashboard widget.",
  },
  addWidgets: {
    id: "app.dashboard.addWidgets",
    defaultMessage: "Add widgets",
    description: "Label for the button that opens the widget drawer.",
  },
  adminBadge: {
    id: "app.dashboard.badge.admin",
    defaultMessage: "Admin",
    description: "Badge indicating an administrator resource scope.",
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
  apiReliabilityLast5Minutes: {
    id: "app.dashboard.apiReliability.last5Minutes",
    defaultMessage: "(last 5 minutes)",
    description:
      "Lookback period shown with the current API reliability metric value.",
  },
  apiRequestRateWidget: {
    id: "app.dashboard.widget.apiRequestRate",
    defaultMessage: "API request rate",
    description: "Title for the API request rate reliability widget.",
  },
  attentionRequiredWidget: {
    id: "app.dashboard.widget.attentionRequired",
    defaultMessage: "Attention required",
    description: "Title for the operational dashboard attention widget.",
  },
  cpus: {
    id: "app.dashboard.summary.cpus",
    defaultMessage: "CPUs",
    description: "Summary label for provisioned CPU capacity.",
  },
  dashboardHelpApiErrorRate: {
    id: "app.dashboard.help.apiErrorRate",
    defaultMessage:
      "The current API error rate is the percentage of API requests that returned an HTTP 5xx response over the last 5 minutes. Each trend point is an hourly sample from the previous 24 hours. Its value is the 5xx error rate calculated from the 5 minutes immediately before that sample's timestamp.<br></br>HTTP 4xx responses, such as 401 or 403 responses, aren't included in the error rate.",
    description: "Help text for the API error rate widget.",
  },
  dashboardHelpApiLatency: {
    id: "app.dashboard.help.apiLatency",
    defaultMessage:
      "The current API latency is the median response time for API requests over the last 5 minutes. Each trend point is an hourly sample from the previous 24 hours. Its value is the median latency calculated from the 5 minutes immediately before that sample's timestamp.<br></br>Latency is shown in seconds. The median means that half of the requests were completed in less time and half took longer.",
    description: "Help text for the API latency widget.",
  },
  dashboardHelpApiRequestRate: {
    id: "app.dashboard.help.apiRequestRate",
    defaultMessage:
      "The current API request rate is the average number of requests per second over the last 5 minutes. Each trend point is an hourly sample from the previous 24 hours. Its value is calculated from the 5 minutes immediately before that sample's timestamp.<br></br>The API request rate includes inbound requests to the HyperShell API endpoints.",
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
  dashboardHelpStaleResourceStatus: {
    id: "app.dashboard.help.staleResourceStatus",
    defaultMessage:
      "The current value is the number of resources whose observed status has not caught up with their desired state. A resource is stale when its observed release is different from the release currently desired by the control plane. Each trend point shows the count of stale resources at that sample time.",
    description: "Help text for the stale resource status widget.",
  },
  dashboardHelpUsers: {
    id: "app.dashboard.help.users",
    defaultMessage:
      "The current Users value is the total number of registered users in HyperShell. It isn't a count of users who are online right now.<br></br>The trend shows the number of distinct registered users who used the API on each UTC calendar day during the last 30 days. A user is counted once per day even if they made multiple API requests.",
    description: "Help text for the Users widget.",
  },
  dashboardInventoryDescription: {
    id: "app.dashboard.inventory.description",
    defaultMessage:
      "Review the managed clusters available across the HyperShell platform.",
    description: "Supporting text on the platform inventory dashboard page.",
  },
  dashboardInventoryPurpose: {
    id: "app.dashboard.inventory.purpose",
    defaultMessage:
      "How large is the managed cluster fleet, and where is it distributed? Use this system-wide view to understand provider and regional coverage.",
    description: "Purpose statement for the platform inventory dashboard.",
  },
  dashboardInventoryPurposeHeading: {
    id: "app.dashboard.inventory.purposeHeading",
    defaultMessage: "What this dashboard answers",
    description: "Heading for the platform inventory purpose statement.",
  },
  dashboardInventoryTitle: {
    id: "app.dashboard.inventory.title",
    defaultMessage: "Platform inventory",
    description: "Main heading on the platform inventory dashboard page.",
  },
  dashboardNavInventory: {
    id: "app.dashboard.nav.inventory",
    defaultMessage: "Inventory",
    description: "Dashboard navigation label for inventory.",
  },
  dashboardNavOverview: {
    id: "app.dashboard.nav.overview",
    defaultMessage: "Overview",
    description: "Dashboard navigation label for the operational overview.",
  },
  dashboardNavProvisioning: {
    id: "app.dashboard.nav.provisioning",
    defaultMessage: "Provisioning",
    description: "Dashboard navigation label for provisioning.",
  },
  dashboardProvisioningDescription: {
    id: "app.dashboard.provisioning.description",
    defaultMessage:
      "Monitor hub-cluster capacity and platform-wide gateway provisioning.",
    description: "Supporting text on the provisioning dashboard page.",
  },
  dashboardProvisioningPurpose: {
    id: "app.dashboard.provisioning.purpose",
    defaultMessage:
      "Is the hub cluster healthy enough to host HyperShell services and provision gateways? Review current utilization, node and pod health, and provisioning reliability.",
    description: "Purpose statement for the provisioning dashboard.",
  },
  dashboardProvisioningPurposeHeading: {
    id: "app.dashboard.provisioning.purposeHeading",
    defaultMessage: "What this dashboard answers",
    description: "Heading for the provisioning dashboard purpose statement.",
  },
  dashboardProvisioningTitle: {
    id: "app.dashboard.provisioning.title",
    defaultMessage: "Provisioning dashboard",
    description: "Main heading on the provisioning dashboard page.",
  },
  description: {
    id: "app.dashboard.description",
    defaultMessage:
      "A compact landing page for the current state of the resources you are allowed to operate.",
    description: "Supporting text on the operational dashboard page.",
  },
  durationDaysHours: {
    id: "app.dashboard.duration.daysHours",
    defaultMessage: "{days}d {hours}h",
    description: "Compact days and hours duration.",
  },
  durationHoursMinutes: {
    id: "app.dashboard.duration.hoursMinutes",
    defaultMessage: "{hours}h {minutes}m",
    description: "Compact hours and minutes duration.",
  },
  durationMinutesSeconds: {
    id: "app.dashboard.duration.minutesSeconds",
    defaultMessage: "{minutes}m {seconds}s",
    description: "Compact minutes and seconds duration.",
  },
  gatewayReleasesHelp: {
    id: "app.dashboard.gatewayReleases.help",
    defaultMessage: "Gateway counts grouped by the resolved release version.",
    description: "Supporting text for the gateway releases widget.",
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
  gatewayStatusDegradedGateways: {
    id: "app.dashboard.gatewayStatus.degradedGateways",
    defaultMessage: "Degraded gateways",
    description: "Attention widget label for degraded gateways.",
  },
  gatewayStatusFailed: {
    id: "app.dashboard.gatewayStatus.failed",
    defaultMessage: "Failed",
    description: "Legend label for failed gateways.",
  },
  gatewayStatusFailedGateways: {
    id: "app.dashboard.gatewayStatus.failedGateways",
    defaultMessage: "Failed gateways",
    description: "Attention widget label for failed gateways.",
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
    defaultMessage: "Gateway status",
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
  lastSuccessfulRefreshShort: {
    id: "app.dashboard.lastSuccessfulRefreshShort",
    defaultMessage: "Last successful refresh",
    description: "Compact refresh timestamp label inside a dashboard widget.",
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
  nodeStatusCardTitle: {
    id: "app.dashboard.nodeStatus.cardTitle",
    defaultMessage: "{ready} / {total}",
    description: "Ready and total node count in the node status widget.",
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
  noResourcesNeedAttention: {
    id: "app.dashboard.attention.none",
    defaultMessage: "No resources need attention",
    description: "Empty state for the attention-required widget.",
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
  percentChangeDecrease: {
    id: "app.dashboard.percentChange.decrease",
    defaultMessage: "-{percent}%",
    description: "Formatted percentage decrease.",
  },
  percentChangeIncrease: {
    id: "app.dashboard.percentChange.increase",
    defaultMessage: "+{percent}%",
    description: "Formatted percentage increase.",
  },
  platformInventoryClustersNeedingAttention: {
    id: "app.dashboard.inventory.clustersNeedingAttention",
    defaultMessage: "Clusters needing attention",
    description: "Managed cluster count not currently reporting Ready.",
  },
  platformInventoryCompleteNote: {
    id: "app.dashboard.inventory.completeNote",
    defaultMessage: "Counts represent the complete platform inventory.",
    description: "Scope note under the platform inventory summary.",
  },
  platformInventoryExceptions: {
    id: "app.dashboard.inventory.exceptions",
    defaultMessage: "Inventory exceptions",
    description:
      "Managed cluster count in a failed, degraded, or pending state.",
  },
  platformInventoryHealthyClusters: {
    id: "app.dashboard.inventory.healthyClusters",
    defaultMessage: "Healthy clusters",
    description: "Managed cluster count currently reporting Ready.",
  },
  platformInventoryManagedClusters: {
    id: "app.dashboard.inventory.managedClusters",
    defaultMessage: "Managed clusters",
    description: "Total managed clusters in the platform inventory.",
  },
  platformInventoryRecentlyAdded: {
    id: "app.dashboard.inventory.recentlyAdded",
    defaultMessage: "Recently added ({days}d)",
    description: "Managed clusters created within the selected time range.",
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
  provisioningCpuUsed: {
    id: "app.dashboard.provisioning.cpuUsed",
    defaultMessage: "CPU used",
    description: "Summary label for hub-cluster CPU utilization.",
  },
  provisioningInProgress: {
    id: "app.dashboard.provisioning.inProgress",
    defaultMessage: "In progress",
    description: "Label for gateways currently provisioning.",
  },
  provisioningManagedClusters: {
    id: "app.dashboard.provisioning.managedClusters",
    defaultMessage: "Managed clusters",
    description: "Summary label for the managed-cluster inventory total.",
  },
  provisioningMedianTime: {
    id: "app.dashboard.provisioning.medianTime",
    defaultMessage: "Median time to ready",
    description: "Label for median gateway provisioning duration.",
  },
  provisioningMemoryUsed: {
    id: "app.dashboard.provisioning.memoryUsed",
    defaultMessage: "Memory used",
    description: "Summary label for hub-cluster memory utilization.",
  },
  provisioningProvisionReliabilityWidget: {
    id: "app.dashboard.widget.provisioningProvisionReliability",
    defaultMessage: "Provision reliability",
    description: "Title for the provisioning dashboard reliability widget.",
  },
  provisioningProvisionTimeWidget: {
    id: "app.dashboard.widget.provisioningProvisionTime",
    defaultMessage: "Provision time",
    description: "Title for the provisioning dashboard duration widget.",
  },
  provisioningScopeNote: {
    id: "app.dashboard.provisioning.scopeNote",
    defaultMessage:
      "Hub-cluster capacity and platform provisioning totals are visible to platform administrators.",
    description: "Scope note under the provisioning system summary widget.",
  },
  provisioningSnapshotHelp: {
    id: "app.dashboard.provisioning.snapshotHelp",
    defaultMessage:
      "Open Provisioning for failure reasons and time-to-ready trends.",
    description: "Guidance displayed in the provisioning snapshot widget.",
  },
  provisioningSnapshotWidget: {
    id: "app.dashboard.widget.provisioningSnapshot",
    defaultMessage: "Provisioning snapshot",
    description: "Title for the operational provisioning snapshot widget.",
  },
  provisioningSuccessful: {
    id: "app.dashboard.provisioning.successful",
    defaultMessage: "Successful",
    description: "Label for successful gateway provisions.",
  },
  provisioningSuccessRateChange: {
    id: "app.dashboard.provisioning.successRateChange",
    defaultMessage: "Success rate change",
    description: "Label for the change in gateway provision success rate.",
  },
  provisioningSuccessRateWidgetAriaLabel: {
    id: "app.dashboard.provisioning.successRateAriaLabel",
    defaultMessage: "Provision reliability",
    description:
      "Accessible label for the provisioning reliability summary list.",
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
  provisionReliabilityDailySuccessRateCaption: {
    id: "app.dashboard.provisionReliability.dailySuccessRateCaption",
    defaultMessage: "Last {days} days - daily success rate",
    description:
      "Caption for the daily provision success-rate sparkline in the selected range.",
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
  provisionReliabilitySuccessRate: {
    id: "app.dashboard.provisionReliability.successRate",
    defaultMessage: "Success rate",
    description: "Title for the provision success-rate sparkline.",
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
  provisionTimeP50Label: {
    id: "app.dashboard.provisionTime.p50",
    defaultMessage: "P50",
    description: "Label for the 50th percentile gateway provision duration.",
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
  reconciliationChartAccumulating: {
    id: "app.dashboard.reconciliationChart.accumulating",
    defaultMessage: "Trend data is accumulating - check back in a few minutes",
    description:
      "Shown in the reconciliation over-time chart when 5-minute data exists but there are too few points to draw a line.",
  },
  reconciliationChartLegendFailures: {
    id: "app.dashboard.reconciliationChart.legend.failures",
    defaultMessage: "Failures",
    description:
      "Legend label for the failures series in the reconciliation over-time chart.",
  },
  reconciliationChartLegendRetries: {
    id: "app.dashboard.reconciliationChart.legend.retries",
    defaultMessage: "Retries",
    description:
      "Legend label for the retries series in the reconciliation over-time chart.",
  },
  reconciliationChartLegendSuccesses: {
    id: "app.dashboard.reconciliationChart.legend.successes",
    defaultMessage: "Successes",
    description:
      "Legend label for the successes series in the reconciliation over-time chart.",
  },
  reconciliationChartNoData: {
    id: "app.dashboard.reconciliationChart.noData",
    defaultMessage: "No trend data available",
    description:
      "Shown in the reconciliation over-time chart when no hourly trend data is available.",
  },
  reconciliationFailuresLast24Hours: {
    id: "app.dashboard.reconciliationFailures.last24Hours",
    defaultMessage: "(last 24 hours)",
    description:
      "Lookback period shown with the current reconciliation failure count.",
  },
  reconciliationLagCurrent: {
    id: "app.dashboard.reconciliationLag.current",
    defaultMessage: "Current P50 lag",
    description:
      "Value label shown with the current reconciliation lag metric.",
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
  reliabilityPurpose: {
    id: "app.dashboard.reliability.purpose",
    defaultMessage:
      "Is the platform serving requests reliably? Are reconciliations failing, retrying, or falling behind? Are there incidents or alerts requiring investigation? These platform-wide operational signals are for system admins; production views should scope them to permitted fleets where supported.",
    description: "Question summary explaining the reliability dashboard.",
  },
  reliabilityPurposeHeading: {
    id: "app.dashboard.reliability.purposeHeading",
    defaultMessage: "What this dashboard answers",
    description: "Heading for the reliability dashboard purpose.",
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
  reliabilitySummaryReconciliationFailures: {
    id: "app.dashboard.reliability.summary.reconciliationFailures",
    defaultMessage: "Reconciliation failures",
    description: "Summary row label for reconciliation failures.",
  },
  reliabilitySummaryReconciliationLag: {
    id: "app.dashboard.reliability.summary.reconciliationLag",
    defaultMessage: "Reconciliation lag",
    description: "Summary row label for reconciliation lag.",
  },
  reliabilitySummaryReconciliationRetries: {
    id: "app.dashboard.reliability.summary.reconciliationRetries",
    defaultMessage: "Reconciliation retries",
    description: "Summary row label for reconciliation retries.",
  },
  reliabilitySummaryReconciliationSuccesses: {
    id: "app.dashboard.reliability.summary.reconciliationSuccesses",
    defaultMessage: "Reconciliation successes",
    description: "Summary row label for successful reconciliations.",
  },
  reliabilitySummaryRequestRate: {
    id: "app.dashboard.reliability.summary.requestRate",
    defaultMessage: "Request rate",
    description: "Summary row label for API request rate.",
  },
  reliabilitySummaryStaleResourceStatus: {
    id: "app.dashboard.reliability.summary.staleResourceStatus",
    defaultMessage: "Stale resource status",
    description: "Summary row label for resources with stale observed status.",
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
    defaultMessage: "Reset to default layout",
    description:
      "Label for restoring the operational dashboard default layout.",
  },
  resourceSummaryHelp: {
    id: "app.dashboard.resourceSummary.help",
    defaultMessage: "Counts respect your fleet and environment access.",
    description: "Scope guidance in the resource summary widget.",
  },
  resourceSummaryWidget: {
    id: "app.dashboard.widget.resourceSummary",
    defaultMessage: "Resource summary",
    description: "Title for the authorized resource summary widget.",
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
  sandboxAttentionExpiringSandboxes: {
    id: "app.dashboard.sandboxAttention.expiringSandboxes",
    defaultMessage: "Expiring sandboxes",
    description: "Summary label for sandboxes expiring within 24 hours.",
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
  sandboxAttentionIdleOrOrphaned: {
    id: "app.dashboard.sandboxAttention.idleOrOrphaned",
    defaultMessage: "Idle or orphaned sandboxes",
    description: "Combined attention label for idle and orphaned sandboxes.",
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
  staleResourceStatusCurrent: {
    id: "app.dashboard.staleResourceStatus.current",
    defaultMessage: "Current stale-resource count",
    description:
      "Value label shown with the current stale resource status metric.",
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
  summaryHealthyResources: {
    id: "app.dashboard.summary.healthyResources",
    defaultMessage: "Healthy resources",
    description: "Summary row for healthy authorized gateways and sandboxes.",
  },
  summaryResourcesNeedingAttention: {
    id: "app.dashboard.summary.resourcesNeedingAttention",
    defaultMessage: "Resources needing attention",
    description: "Summary row for authorized resources needing attention.",
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
  systemScopeLabel: {
    id: "app.dashboard.systemScopeLabel",
    defaultMessage: "System",
    description: "Badge identifying a system-wide dashboard widget.",
  },
  systemSummaryWidget: {
    id: "app.dashboard.widget.systemSummary",
    defaultMessage: "System summary",
    description: "Title for the operational dashboard system summary widget.",
  },
  timeRange: {
    id: "app.dashboard.timeRange.label",
    defaultMessage: "Time range",
    description: "Accessible label for the operational dashboard time range.",
  },
  timeRangeLast7Days: {
    id: "app.dashboard.timeRange.last7Days",
    defaultMessage: "Last 7 days",
    description: "Seven-day operational dashboard range option.",
  },
  timeRangeLast30Days: {
    id: "app.dashboard.timeRange.last30Days",
    defaultMessage: "Last 30 days",
    description: "Thirty-day operational dashboard range option.",
  },
  title: {
    id: "app.dashboard.title",
    defaultMessage: "Operational overview",
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
  updatedMinuteAgo: {
    id: "app.dashboard.updatedMinuteAgo",
    defaultMessage: "Updated 1 minute ago",
    description:
      "Relative timestamp shown when the dashboard was updated one minute ago.",
  },
  updatedMinutesAgo: {
    id: "app.dashboard.updatedMinutesAgo",
    defaultMessage: "Updated {minutes} minutes ago",
    description:
      "Relative timestamp shown beside the dashboard refresh action.",
  },
  updatedUnderMinuteAgo: {
    id: "app.dashboard.updatedUnderMinuteAgo",
    defaultMessage: "Updated under 1 minute ago",
    description:
      "Relative timestamp shown when the dashboard was updated less than one minute ago.",
  },
  usageActiveSandboxes: {
    id: "app.dashboard.usage.sandboxUtilization.active",
    defaultMessage: "Active sandboxes",
    description: "Label for the active sandbox count.",
  },
  usageActiveShare: {
    id: "app.dashboard.usage.activeGateways.activeShare",
    defaultMessage: "Active share",
    description: "Label for the active gateway share.",
  },
  usageDescription: {
    id: "app.dashboard.usage.description",
    defaultMessage:
      "See how gateways and sandboxes are being used, not just how many have been provisioned.",
    description: "Supporting text on the usage dashboard page.",
  },
  usageErrorRateSubtitle: {
    id: "app.dashboard.usage.errorRate.subtitle",
    defaultMessage: "Errors as a percentage of requests",
    description: "Subtitle for the gateway request error-rate value.",
  },
  usageLoadErrorBody: {
    id: "app.dashboard.usage.loadError.body",
    defaultMessage:
      "An unexpected error occurred while loading usage metrics. Try refreshing the dashboard.",
    description:
      "Recovery guidance when usage dashboard metrics cannot be loaded.",
  },
  usageLoadErrorTitle: {
    id: "app.dashboard.usage.loadError.title",
    defaultMessage: "Usage dashboard metrics are unavailable",
    description: "Title shown when usage dashboard metrics cannot be loaded.",
  },
  usageLoading: {
    id: "app.dashboard.usage.loading",
    defaultMessage: "Loading usage dashboard metrics",
    description: "Accessible status shown while usage metrics load.",
  },
  usageNavUsage: {
    id: "app.dashboard.nav.usage",
    defaultMessage: "Usage",
    description: "Secondary navigation label for the usage dashboard route.",
  },
  usagePartialDataBody: {
    id: "app.dashboard.usage.partialData.body",
    defaultMessage:
      "The dashboard shows the data that is currently available. Missing daily points are not treated as zero.",
    description: "Guidance shown when usage trend data is incomplete.",
  },
  usagePartialDataLabel: {
    id: "app.dashboard.usage.partialData.label",
    defaultMessage: "Partial data",
    description: "Label shown inside a widget with incomplete trend data.",
  },
  usagePartialDataTitle: {
    id: "app.dashboard.usage.partialData.title",
    defaultMessage: "Some usage data is incomplete",
    description: "Title shown when usage data covers only part of the range.",
  },
  usageProvisionedGateways: {
    id: "app.dashboard.usage.activeGateways.provisioned",
    defaultMessage: "Provisioned gateways",
    description: "Label for the authorized provisioned gateway count.",
  },
  usageProvisionedSandboxes: {
    id: "app.dashboard.usage.sandboxUtilization.provisioned",
    defaultMessage: "Provisioned sandboxes",
    description: "Label for the provisioned sandbox count.",
  },
  usagePurpose: {
    id: "app.dashboard.usage.purpose",
    defaultMessage:
      "Are my gateways receiving traffic? Which provisioned gateways are unused? How are my sandboxes being utilized, and how long do they live?",
    description: "Purpose statement for the usage dashboard.",
  },
  usagePurposeAuthorizedScope: {
    id: "app.dashboard.usage.purpose.authorizedScope",
    defaultMessage: "Metrics are scoped to the resources you can access.",
    description: "Resource scope note shown to non-administrators.",
  },
  usagePurposeHeading: {
    id: "app.dashboard.usage.purposeHeading",
    defaultMessage: "What this dashboard answers",
    description: "Heading for the usage dashboard purpose statement.",
  },
  usagePurposeSystemScope: {
    id: "app.dashboard.usage.purpose.systemScope",
    defaultMessage: "Metrics cover the entire fleet.",
    description: "Fleet scope note shown to platform administrators.",
  },
  usageRequestVolumeSubtitle: {
    id: "app.dashboard.usage.requestVolume.subtitle",
    defaultMessage: "Requests in the selected range",
    description: "Subtitle for the gateway request volume value.",
  },
  usageSandboxLifetimeMean: {
    id: "app.dashboard.usage.sandboxLifetime.mean",
    defaultMessage: "Average sandbox lifetime",
    description: "Label for the mean sandbox lifetime value.",
  },
  usageSandboxLifetimeP50: {
    id: "app.dashboard.usage.sandboxLifetime.p50",
    defaultMessage: "P50 lifetime",
    description: "Label for the median sandbox lifetime value.",
  },
  usageSandboxLifetimeP95: {
    id: "app.dashboard.usage.sandboxLifetime.p95",
    defaultMessage: "P95 lifetime",
    description: "Label for the 95th-percentile sandbox lifetime value.",
  },
  usageSummaryWidget: {
    id: "app.dashboard.widget.usageSummary",
    defaultMessage: "Usage summary",
    description: "Title for the operational dashboard usage summary widget.",
  },
  usageTitle: {
    id: "app.dashboard.usage.title",
    defaultMessage: "Usage dashboard",
    description: "Main heading on the usage dashboard page.",
  },
  usageUnusedGatewaysHelp: {
    id: "app.dashboard.usage.unusedGateways.help",
    defaultMessage:
      "Gateways with no qualifying request activity in the selected period.",
    description: "Guidance in the unused gateway usage widget.",
  },
  usageWidgetActiveGateways: {
    id: "app.dashboard.usage.widget.activeGateways",
    defaultMessage: "Active gateways",
    description: "Title for the active gateway usage widget.",
  },
  usageWidgetGatewayErrorRate: {
    id: "app.dashboard.usage.widget.gatewayErrorRate",
    defaultMessage: "Gateway error rate",
    description: "Title for the gateway request error-rate usage widget.",
  },
  usageWidgetGatewayRequestVolume: {
    id: "app.dashboard.usage.widget.gatewayRequestVolume",
    defaultMessage: "Gateway request volume",
    description: "Title for the gateway request volume usage widget.",
  },
  usageWidgetSandboxLifetime: {
    id: "app.dashboard.usage.widget.sandboxLifetime",
    defaultMessage: "Sandbox lifetime",
    description: "Title for the sandbox lifetime usage widget.",
  },
  usageWidgetSandboxUtilization: {
    id: "app.dashboard.usage.widget.sandboxUtilization",
    defaultMessage: "Sandbox utilization",
    description: "Title for the sandbox utilization usage widget.",
  },
  usageWidgetUnusedGateways: {
    id: "app.dashboard.usage.widget.unusedGateways",
    defaultMessage: "Provisioned but unused",
    description: "Title for the unused gateway usage widget.",
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
  widgetReconciliationFailures: {
    id: "app.dashboard.widget.reconciliationFailures",
    defaultMessage: "Reconciliation failures",
    description: "Title for the reconciliation failures dashboard widget.",
  },
  widgetReconciliationLag: {
    id: "app.dashboard.widget.reconciliationLag",
    defaultMessage: "Reconciliation lag",
    description: "Title for the reconciliation lag dashboard widget.",
  },
  widgetReconciliationOverTime: {
    id: "app.dashboard.widget.reconciliationOverTime",
    defaultMessage: "Reconciliations over time",
    description:
      "Title for the reconciliation over-time multi-line chart widget.",
  },
  widgetReconciliationRetries: {
    id: "app.dashboard.widget.reconciliationRetries",
    defaultMessage: "Reconciliation retries",
    description: "Title for the reconciliation retries dashboard widget.",
  },
  widgetReconciliationSuccesses: {
    id: "app.dashboard.widget.reconciliationSuccesses",
    defaultMessage: "Reconciliation successes",
    description: "Title for the reconciliation successes dashboard widget.",
  },
  widgetSandboxes: {
    id: "app.dashboard.summary.sandboxes",
    defaultMessage: "Sandboxes",
    description:
      "Label for provisioned sandboxes on the operational dashboard.",
  },
  widgetStaleResourceStatus: {
    id: "app.dashboard.widget.staleResourceStatus",
    defaultMessage: "Stale resource status",
    description: "Title for the stale resource status dashboard widget.",
  },
});

export const dashboardHelpMessages: Partial<
  Record<ReliabilityMetricId, MessageDescriptor>
> = {
  "api-error-rate": messages.dashboardHelpApiErrorRate,
  "api-latency": messages.dashboardHelpApiLatency,
  "api-request-rate": messages.dashboardHelpApiRequestRate,
  "reconciliation-failures": messages.dashboardHelpReconciliationFailures,
  "reconciliation-lag": messages.dashboardHelpReconciliationLag,
  "reconciliation-retries": messages.dashboardHelpReconciliationRetries,
  "stale-resource-status-count": messages.dashboardHelpStaleResourceStatus,
};

export function getDashboardHelpMessage(
  metricId: string,
): MessageDescriptor | undefined {
  if (!(metricId in dashboardHelpMessages)) {
    return undefined;
  }

  return dashboardHelpMessages[metricId as ReliabilityMetricId];
}
