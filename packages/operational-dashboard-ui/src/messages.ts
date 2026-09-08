import { defineMessages } from "react-intl";

export const messages = defineMessages({
  addWidgets: {
    id: "app.dashboard.addWidgets",
    defaultMessage: "Add widgets",
    description: "Label for the button that opens the widget drawer.",
  },
  cpus: {
    id: "app.dashboard.summary.cpus",
    defaultMessage: "CPUs",
    description: "Summary label for provisioned CPU capacity.",
  },
  description: {
    id: "app.dashboard.description",
    defaultMessage:
      "Live view of gateway fleet health, hub cluster capacity, and platform usage. Metrics refresh every 15 minutes; use Refresh to update now.",
    description: "Supporting text on the operational dashboard page.",
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
    defaultMessage: "Gateway status",
    description: "Title for the gateway status dashboard widget.",
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
    defaultMessage: "Registered users",
    description: "Title for the registered users dashboard widget.",
  },
  registeredUsersSummary: {
    id: "app.dashboard.summary.registeredUsers",
    defaultMessage: "Registered users",
    description: "Summary label for registered users.",
  },
  resetToDefault: {
    id: "app.dashboard.resetToDefault",
    defaultMessage: "Reset to default",
    description:
      "Label for restoring the operational dashboard default layout.",
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
    defaultMessage: "{percent}% decrease",
    description:
      "Tooltip for a usage summary metric that decreased since the start of its trend.",
  },
  summaryTrendIncrease: {
    id: "app.dashboard.summary.trendIncrease",
    defaultMessage: "{percent}% increase",
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
    defaultMessage: "HyperShell operational dashboard",
    description: "Main heading on the operational dashboard page.",
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
