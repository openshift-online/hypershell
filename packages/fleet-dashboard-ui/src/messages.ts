// Central message catalog. Keys are sorted (enforced by ESLint) and every entry
// carries an explicit id + defaultMessage + description
// (formatjs enforce-id/enforce-default-message/enforce-description).
// UI copy carries NO fleet identity - instance/environment names are data, not strings.

import { defineMessages } from "react-intl";

export const messages = defineMessages({
  agentActive: {
    id: "fleet.agents.activity.active",
    defaultMessage: "Active",
    description:
      "Agent liveness state: the agent is running right now (SDLC-AGT-17).",
  },
  agentLastSeen: {
    id: "fleet.agents.activity.lastSeen",
    defaultMessage: "last seen {ago} ago",
    description:
      "Relative time since an agent last reported, shown beside its activity state (SDLC-AGT-17).",
  },
  agentMonitoringBlind: {
    id: "fleet.agents.activity.blind",
    defaultMessage: "Monitoring blind",
    description:
      "Agent liveness state: the telemetry plane is stale, so liveness cannot be asserted (SDLC-AGT-17).",
  },
  agentNotActive: {
    id: "fleet.agents.activity.notActive",
    defaultMessage: "Not active",
    description:
      "Agent liveness state: the agent is not running right now (the normal resting state of a cron agent) (SDLC-AGT-17).",
  },
  agentsColActivity: {
    id: "fleet.agents.col.activity",
    defaultMessage: "Activity",
    description: "Agents table column header for the agent activity state.",
  },
  agentsColAgent: {
    id: "fleet.agents.col.agent",
    defaultMessage: "Agent",
    description: "Agents table column header for the agent name.",
  },
  agentsColDone: {
    id: "fleet.agents.col.done",
    defaultMessage: "Done",
    description: "Agents table column header for items processed.",
  },
  agentsColFailed: {
    id: "fleet.agents.col.failed",
    defaultMessage: "Failed",
    description: "Agents table column header for items failed.",
  },
  agentsColQueue: {
    id: "fleet.agents.col.queue",
    defaultMessage: "Queue",
    description: "Agents table column header for queue depth.",
  },
  agentsColReplicas: {
    id: "fleet.agents.col.replicas",
    defaultMessage: "Replicas",
    description: "Agents table column header for replica count.",
  },
  agentsEmptyForCluster: {
    id: "fleet.agents.emptyForCluster",
    defaultMessage: "No SDLC agents are reporting on this cluster.",
    description:
      "Shown in the Agents tab when no agents are reporting for this cluster.",
  },
  agentsTableAria: {
    id: "fleet.agents.tableAria",
    defaultMessage: "SDLC agents on this cluster",
    description: "Accessible name for the per-cluster agents table.",
  },
  appTitle: {
    id: "fleet.app.title",
    defaultMessage: "HyperShell Operational Status",
    description: "Dashboard page and masthead title.",
  },
  bundleNoPrs: {
    id: "fleet.map.bundle.noPrs",
    defaultMessage: "No pull requests in this build.",
    description:
      "Shown in the Bundle tab when a release has no associated PRs.",
  },
  bundlePrSummary: {
    id: "fleet.map.bundle.prSummary",
    defaultMessage:
      "{count, plural, one {# PR} other {# PRs}} · since previous build",
    description: "Count of pull requests included in a release bundle.",
  },
  columnEnvironment: {
    id: "fleet.promotion.column.environment",
    defaultMessage: "Environment",
    description: "Promotion table column header for the environment name.",
  },
  columnGates: {
    id: "fleet.promotion.column.gates",
    defaultMessage: "Gates",
    description: "Promotion table column header for the gate status.",
  },
  columnGoverning: {
    id: "fleet.promotion.column.governing",
    defaultMessage: "Governing instance",
    description:
      "Promotion table column header for the instance governing an environment.",
  },
  columnHealth: {
    id: "fleet.instances.column.health",
    defaultMessage: "Health",
    description: "Instances table column header for the health status.",
  },
  columnInstance: {
    id: "fleet.instances.column.instance",
    defaultMessage: "Instance",
    description: "Instances table column header for the instance name.",
  },
  columnMetricFailLabel: {
    id: "fleet.agents.columnMetric.failLabel",
    defaultMessage: "fail",
    description:
      "Compact lowercase label for the failed-items count in the SDLC board column runtime strip.",
  },
  columnMetricOnLabel: {
    id: "fleet.agents.columnMetric.onLabel",
    defaultMessage: "on",
    description:
      "Compact lowercase label for the active-replica count in the SDLC board column runtime strip.",
  },
  columnMetricQueueLabel: {
    id: "fleet.agents.columnMetric.queueLabel",
    defaultMessage: "queue",
    description:
      "Compact lowercase label for the queue-depth count in the SDLC board column runtime strip.",
  },
  columnMetricRatio: {
    id: "fleet.agents.columnMetric.ratio",
    defaultMessage: "{on}/{total}",
    description:
      "Compact active/total replica ratio in the SDLC board column runtime strip.",
  },
  columnProvider: {
    id: "fleet.instances.column.provider",
    defaultMessage: "Provider",
    description: "Instances table column header for the cloud provider.",
  },
  columnRegion: {
    id: "fleet.instances.column.region",
    defaultMessage: "Region",
    description: "Instances table column header for the region.",
  },
  columnRelease: {
    id: "fleet.promotion.column.release",
    defaultMessage: "Active release",
    description: "Promotion table column header for the active release.",
  },
  columnRole: {
    id: "fleet.instances.column.role",
    defaultMessage: "Role",
    description: "Instances table column header for the instance role.",
  },
  detailAlias: {
    id: "fleet.map.detail.alias",
    defaultMessage: "Alias",
    description: "Detail label for a release bundle's generated alias name.",
  },
  detailAnalysisLogs: {
    id: "fleet.map.detail.analysisLogs",
    defaultMessage: "Analysis logs",
    description:
      "Detail label for the link to a gate's analysis logs, viewable in the Argo CD application tree.",
  },
  detailAnalysisRun: {
    id: "fleet.map.detail.analysisRun",
    defaultMessage: "Analysis run",
    description:
      "Detail label for the link to a gate's end-to-end analysis run status report.",
  },
  detailCluster: {
    id: "fleet.map.detail.cluster",
    defaultMessage: "Cluster",
    description: "Detail label for the cluster backing an instance.",
  },
  detailClusters: {
    id: "fleet.map.detail.clusters",
    defaultMessage: "Managed clusters",
    description: "Detail label for the number of clusters an instance manages.",
  },
  detailDate: {
    id: "fleet.map.detail.date",
    defaultMessage: "Released",
    description: "Detail label for a release bundle's release date.",
  },
  detailDeployments: {
    id: "fleet.map.detail.deployments",
    defaultMessage: "Deployed to",
    description: "Detail label listing the instances running a release bundle.",
  },
  detailDigest: {
    id: "fleet.map.detail.digest",
    defaultMessage: "Digest",
    description: "Detail label for a release bundle's image digest.",
  },
  detailDrift: {
    id: "fleet.map.detail.drift",
    defaultMessage: "Version drift",
    description:
      "Detail label flagging that an instance runs a different release than its environment's hub.",
  },
  detailFinalStage: {
    id: "fleet.map.detail.finalStage",
    defaultMessage: "final stage",
    description:
      "Shown as a promotion gate's destination when it is the last stage (nothing downstream).",
  },
  detailFlow: {
    id: "fleet.map.detail.flow",
    defaultMessage: "Flow",
    description: "Detail label for a promotion gate's source and destination.",
  },
  detailGatewayBreakdown: {
    id: "fleet.map.detail.gatewayBreakdown",
    defaultMessage:
      "{total} total · {running} running · {provisioning} provisioning · {failed} failed",
    description: "Breakdown of an instance's gateway counts by phase.",
  },
  detailGatewayImage: {
    id: "fleet.map.detail.gatewayImage",
    defaultMessage: "Default gateway",
    description:
      "Detail label for the default OpenShell gateway image the cluster's control plane provisions.",
  },
  detailLinks: {
    id: "fleet.map.detail.links",
    defaultMessage: "Links",
    description: "Detail label for an instance's external deep links.",
  },
  detailMetrics: {
    id: "fleet.map.detail.metrics",
    defaultMessage: "Latency (p95)",
    description: "Detail label for an instance's p95 latency metrics.",
  },
  detailMetricTriple: {
    id: "fleet.map.detail.metricTriple",
    defaultMessage: "RPC {rpc}ms · reconcile {reconcile}ms · BFF {bff}ms",
    description: "The three p95 latency figures for an instance.",
  },
  detailPromoting: {
    id: "fleet.map.detail.promoting",
    defaultMessage: "Promoting",
    description: "Detail label indicating a gate is actively promoting.",
  },
  detailPromotion: {
    id: "fleet.map.detail.promotion",
    defaultMessage: "Promotion",
    description: "Detail label for an instance's promotion state.",
  },
  detailProposed: {
    id: "fleet.map.detail.proposed",
    defaultMessage: "Proposed release",
    description: "Detail label for the release proposed for an instance.",
  },
  detailRelease: {
    id: "fleet.map.detail.release",
    defaultMessage: "Release",
    description: "Detail label for a release bundle's version.",
  },
  detailSync: {
    id: "fleet.map.detail.sync",
    defaultMessage: "Sync status",
    description: "Detail label for an instance's Argo CD sync status.",
  },
  detailSyncAttempts: {
    id: "fleet.map.detail.syncAttempts",
    defaultMessage: "Sync attempts",
    description:
      "Detail label for the Argo CD sync-operation retry count of a gate's source environment.",
  },
  drawerClose: {
    id: "fleet.map.drawer.close",
    defaultMessage: "Close details",
    description: "Accessible label for the detail panel's close button.",
  },
  driftFromHub: {
    id: "fleet.map.drift.fromHub",
    defaultMessage: "Differs from hub",
    description:
      "Badge value shown when an instance runs a different release bundle than its environment's hub.",
  },
  e2eAgo: {
    id: "fleet.e2e.ago",
    defaultMessage:
      "{unit, select, m {{count}m ago} h {{count}h ago} d {{count}d ago} other {just now}}",
    description:
      "Relative time since an end-to-end run started (minutes, hours, days, or just now).",
  },
  e2eAttemptLatest: {
    id: "fleet.e2eAttemptLatest",
    defaultMessage: "Latest (live)",
    description:
      "Attempt selector option in the gate execution panel that follows the current (live) run instead of a pinned historical attempt.",
  },
  e2eAttemptOption: {
    id: "fleet.e2eAttemptOption",
    defaultMessage: "{id} · {started}",
    description: "Execution context e2eAttemptOption",
  },
  e2eAttempts: {
    id: "fleet.e2eAttempts",
    defaultMessage: "Test attempt",
    description: "E2E execution panel e2eAttempts",
  },
  e2eAttemptShort: {
    id: "fleet.e2eAttemptShort",
    defaultMessage: "Attempt {id}",
    description: "E2E execution panel e2eAttemptShort",
  },
  e2eBackendUnavailable: {
    id: "fleet.e2e.backendUnavailable",
    defaultMessage: "Run details are unavailable right now.",
    description:
      "Degraded state shown when the e2e backend (Loki source) cannot be reached.",
  },
  e2eBundleUnknown: {
    id: "fleet.e2eBundleUnknown",
    defaultMessage: "Bundle unavailable",
    description: "E2E run bundle and target context.",
  },
  e2eCollapseLogs: {
    id: "fleet.e2eCollapseLogs",
    defaultMessage: "Collapse",
    description: "E2E execution panel e2eCollapseLogs",
  },
  e2eColumnAttempt: {
    id: "fleet.e2e.column.attempt",
    defaultMessage: "Attempt",
    description: "Recent-runs table column header for the run attempt id.",
  },
  e2eColumnBundle: {
    id: "fleet.e2eColumnBundle",
    defaultMessage: "Bundle",
    description: "E2E run bundle and target context.",
  },
  e2eColumnDuration: {
    id: "fleet.e2e.column.duration",
    defaultMessage: "Duration",
    description: "Table column header for a run or step duration.",
  },
  e2eColumnRevision: {
    id: "fleet.e2e.column.revision",
    defaultMessage: "Revision",
    description:
      "Recent-runs table column header for the hydrated/deployed revision.",
  },
  e2eColumnStarted: {
    id: "fleet.e2e.column.started",
    defaultMessage: "Started",
    description: "Recent-runs table column header for the run start time.",
  },
  e2eColumnState: {
    id: "fleet.e2e.column.state",
    defaultMessage: "State",
    description: "Steps table column header for a step's reduced state.",
  },
  e2eColumnStep: {
    id: "fleet.e2e.column.step",
    defaultMessage: "Step",
    description: "Steps table column header for the step name.",
  },
  e2eColumnTarget: {
    id: "fleet.e2eColumnTarget",
    defaultMessage: "Target",
    description: "E2E run bundle and target context.",
  },
  e2eColumnVerdict: {
    id: "fleet.e2e.column.verdict",
    defaultMessage: "Verdict",
    description: "Recent-runs table column header for a run's overall verdict.",
  },
  e2eDisabled: {
    id: "fleet.e2e.disabled",
    defaultMessage: "End-to-end run reporting is not enabled.",
    description:
      "Feature-off state shown when the BFF reports the e2e feature disabled.",
  },
  e2eDuration: {
    id: "fleet.e2e.duration",
    defaultMessage: "{minutes}m {seconds}s",
    description: "Compact duration of a run or step, in minutes and seconds.",
  },
  e2eExecutionTabs: {
    id: "fleet.e2eExecutionTabs",
    defaultMessage: "Test execution",
    description: "E2E execution panel e2eExecutionTabs",
  },
  e2eExpandLogs: {
    id: "fleet.e2eExpandLogs",
    defaultMessage: "Expand",
    description: "E2E execution panel e2eExpandLogs",
  },
  e2eFlow: {
    id: "fleet.e2eFlow",
    defaultMessage: "{from} → {to}",
    description: "Execution context e2eFlow",
  },
  e2eFollow: {
    id: "fleet.e2e.follow",
    defaultMessage: "Follow",
    description: "Button that resumes following (auto-scrolling) the log pane.",
  },
  e2eGateHistory: {
    id: "fleet.e2eGateHistory",
    defaultMessage: "View run history",
    description: "E2E execution panel e2eGateHistory",
  },
  e2eGateNewAttempt: {
    id: "fleet.e2eGateNewAttempt",
    defaultMessage:
      "A newer attempt is available. Select it above to follow it.",
    description: "E2E execution panel e2eGateNewAttempt",
  },
  e2eGateNoRun: {
    id: "fleet.e2eGateNoRun",
    defaultMessage: "Waiting for a test run for this revision.",
    description: "E2E execution panel e2eGateNoRun",
  },
  e2eGateNoTarget: {
    id: "fleet.e2eGateNoTarget",
    defaultMessage: "The exact verification revision is not available yet.",
    description: "E2E execution panel e2eGateNoTarget",
  },
  e2eGateSummary: {
    id: "fleet.e2eGateSummary",
    defaultMessage: "Promotion gate",
    description: "E2E execution panel e2eGateSummary",
  },
  e2eHistoryTruncated: {
    id: "fleet.e2e.historyTruncated",
    defaultMessage: "Older runs are not shown - run history is truncated.",
    description:
      "Banner shown when more runs exist in the window than the list returns.",
  },
  e2eJumpToLatest: {
    id: "fleet.e2e.log.jumpToLatest",
    defaultMessage: "Jump to latest",
    description:
      "Button that re-pins the log pane to the newest line and resumes auto-follow after the operator scrolled up.",
  },
  e2eLiveDisconnected: {
    id: "fleet.e2e.live.disconnected",
    defaultMessage: "Live updates disconnected - reconnecting.",
    description:
      "Degraded state shown when the live stream for a run is disconnected.",
  },
  e2eLiveGap: {
    id: "fleet.e2e.live.gap",
    defaultMessage:
      "Gap in live updates - {count, plural, one {# entry} other {# entries}} dropped.",
    description:
      "Degraded state shown when the live stream reports dropped entries.",
  },
  e2eLiveLabel: {
    id: "fleet.e2e.live.label",
    defaultMessage: "Live",
    description: "Marker shown while a run's live stream is connected.",
  },
  e2eLogPaneAria: {
    id: "fleet.e2e.log.paneAria",
    defaultMessage: "Run log output",
    description: "Accessible name for the scrollable log output region.",
  },
  e2eLogPaneTitle: {
    id: "fleet.e2e.log.paneTitle",
    defaultMessage: "Logs",
    description: "Heading above the inline log pane.",
  },
  e2eLogScope: {
    id: "fleet.e2e.log.scope",
    defaultMessage: "Log scope",
    description:
      "Accessible name for the toggle choosing whole-run vs selected-step logs.",
  },
  e2eLogScopeRun: {
    id: "fleet.e2e.log.scopeRun",
    defaultMessage: "Whole run",
    description: "Log-scope toggle option showing the whole run's logs.",
  },
  e2eLogScopeStep: {
    id: "fleet.e2e.log.scopeStep",
    defaultMessage: "Selected step",
    description: "Log-scope toggle option showing the selected step's logs.",
  },
  e2eLogsEmpty: {
    id: "fleet.e2e.log.empty",
    defaultMessage: "No log output.",
    description: "Shown when the selected scope has no log lines.",
  },
  e2eLogsEndOfHistory: {
    id: "fleet.e2e.log.endOfHistory",
    defaultMessage: "Beginning of log history.",
    description:
      "Shown at the top of the log pane when all available history for the scope has been loaded (E2E-LOG-26).",
  },
  e2eLogsFailed: {
    id: "fleet.e2e.log.failed",
    defaultMessage: "Could not load log history.",
    description:
      "Shown when a log-history request failed or returned stale; paired with a Retry action instead of a stuck loading indicator (E2E-LOG-26).",
  },
  e2eLogsLimited: {
    id: "fleet.e2e.log.limited",
    defaultMessage: "Showing the most recent log lines held in memory.",
    description:
      "Shown when the in-memory buffer ceiling is reached, distinct from the true end of history (E2E-LOG-26).",
  },
  e2eLogsLoadingOlder: {
    id: "fleet.e2e.log.loadingOlder",
    defaultMessage: "Loading older log lines",
    description:
      "Accessible label for the spinner shown while an older page of log lines is being fetched by infinite scroll.",
  },
  e2eLogsRetry: {
    id: "fleet.e2e.log.retry",
    defaultMessage: "Retry",
    description:
      "Label for the action that re-attempts a failed log-history request (E2E-LOG-26).",
  },
  e2eLogsTruncated: {
    id: "fleet.e2e.log.truncated",
    defaultMessage: "Log output is truncated.",
    description: "Shown when a run produced more log lines than are retained.",
  },
  e2eNoRuns: {
    id: "fleet.e2e.noRuns",
    defaultMessage: "No recent runs.",
    description: "Shown when the recent-runs list is empty.",
  },
  e2eNoSteps: {
    id: "fleet.e2e.noSteps",
    defaultMessage: "No steps reported yet.",
    description: "Shown when a structured run has not yet reported any steps.",
  },
  e2eNoStructuredSteps: {
    id: "fleet.e2e.noStructuredSteps",
    defaultMessage: "No structured steps",
    description:
      "Marker for a pre-reporting run with logs but no structured step events (not a stepless pass).",
  },
  e2ePause: {
    id: "fleet.e2e.pause",
    defaultMessage: "Pause",
    description: "Button that pauses following (auto-scrolling) the log pane.",
  },
  e2eProgressTab: {
    id: "fleet.e2eProgressTab",
    defaultMessage: "Progress",
    description: "E2E execution panel e2eProgressTab",
  },
  e2eReportingIncomplete: {
    id: "fleet.e2e.reportingIncomplete",
    defaultMessage: "Reporting incomplete - no final result was recorded.",
    description:
      "Degraded state shown when a run has no terminal record (unknown verdict).",
  },
  e2eResizeDetails: {
    id: "fleet.e2eResizeDetails",
    defaultMessage: "Resize run details",
    description: "Accessible label for the run history drawer splitter.",
  },
  e2eRetries: {
    id: "fleet.e2eRetries",
    defaultMessage: "Sync attempts: {count}",
    description: "Execution context e2eRetries",
  },
  e2eRunDuration: {
    id: "fleet.e2e.runDuration",
    defaultMessage: "Duration {duration}",
    description: "Label for the selected run's overall duration.",
  },
  e2eRunsAria: {
    id: "fleet.e2e.runsAria",
    defaultMessage: "Recent end-to-end runs",
    description: "Accessible name for the recent-runs table.",
  },
  e2eRunSelect: {
    id: "fleet.e2e.runSelect",
    defaultMessage: "Open run {revision}, attempt {attempt}",
    description: "Accessible name for a recent-run selection link.",
  },
  e2eSearchLogs: {
    id: "fleet.e2eSearchLogs",
    defaultMessage: "Search logs",
    description: "E2E execution panel e2eSearchLogs",
  },
  e2eSearchRuns: {
    id: "fleet.e2eSearchRuns",
    defaultMessage: "Search instances, revisions, or attempts",
    description: "E2E execution panel e2eSearchRuns",
  },
  e2eSection: {
    id: "fleet.e2e.section",
    defaultMessage: "End-to-end tests",
    description: "Card title for the E2E run tab content.",
  },
  e2eSelectRun: {
    id: "fleet.e2e.selectRun",
    defaultMessage: "Select a run to see its steps and logs.",
    description:
      "Placeholder shown in the detail pane when no run is selected.",
  },
  e2eSourceRevision: {
    id: "fleet.e2e.sourceRevision",
    defaultMessage: "Source {revision}",
    description:
      "Label for a run's application source revision, shown separately from the deployed revision.",
  },
  e2eStateFailed: {
    id: "fleet.e2e.state.failed",
    defaultMessage: "Failed",
    description: "Step state: the step failed.",
  },
  e2eStateNotRun: {
    id: "fleet.e2e.state.notRun",
    defaultMessage: "Not run",
    description: "Step state: a fail-fast step that was never reached.",
  },
  e2eStatePassed: {
    id: "fleet.e2e.state.passed",
    defaultMessage: "Passed",
    description: "Step state: the step passed.",
  },
  e2eStateRunning: {
    id: "fleet.e2e.state.running",
    defaultMessage: "Running",
    description: "Step state: the step is currently running.",
  },
  e2eStateSkipped: {
    id: "fleet.e2e.state.skipped",
    defaultMessage: "Skipped",
    description: "Step state: a mode-skipped step (distinct from not-run).",
  },
  e2eStepsAria: {
    id: "fleet.e2e.stepsAria",
    defaultMessage: "Run steps",
    description: "Accessible name for the run's steps table.",
  },
  e2eStepsTitle: {
    id: "fleet.e2e.stepsTitle",
    defaultMessage: "Steps",
    description: "Heading above the run's ordered steps.",
  },
  e2eTabTitle: {
    id: "fleet.e2e.tab.title",
    defaultMessage: "E2E Runs",
    description: "Tab label for the E2E run view.",
  },
  e2eTargetLocation: {
    id: "fleet.e2eTargetLocation",
    defaultMessage: "{cluster} / {namespace}",
    description: "Cluster and namespace of a test target.",
  },
  e2eTestFailed: {
    id: "fleet.e2e.testFailed",
    defaultMessage: "This run failed.",
    description: "Degraded state shown when a run's verdict is failed.",
  },
  e2eTimestamps: {
    id: "fleet.e2eTimestamps",
    defaultMessage: "Timestamps",
    description: "E2E execution panel e2eTimestamps",
  },
  e2eVerdictFailed: {
    id: "fleet.e2e.verdict.failed",
    defaultMessage: "Failed",
    description: "Run verdict: the run failed.",
  },
  e2eVerdictPassed: {
    id: "fleet.e2e.verdict.passed",
    defaultMessage: "Passed",
    description: "Run verdict: the run passed.",
  },
  e2eVerdictUnknown: {
    id: "fleet.e2e.verdict.unknown",
    defaultMessage: "Unknown",
    description: "Run verdict: no terminal record, so the result is unknown.",
  },
  emptyPlane: {
    id: "fleet.plane.empty",
    defaultMessage: "No data reported.",
    description: "Shown when a data source returns no rows.",
  },
  errorPlane: {
    id: "fleet.plane.error",
    defaultMessage: "Could not load this view.",
    description: "Shown when a data source fails to load.",
  },
  freightDeployed: {
    id: "fleet.map.freight.deployed",
    defaultMessage: "{count, plural, one {# deployment} other {# deployments}}",
    description: "Count of instances currently running a release bundle.",
  },
  freshnessAging: {
    id: "fleet.freshness.aging",
    defaultMessage: "Updated a while ago",
    description: "Freshness indicator when data is aging but not yet stale.",
  },
  freshnessFresh: {
    id: "fleet.freshness.fresh",
    defaultMessage: "Up to date",
    description: "Freshness indicator when data is current.",
  },
  freshnessStale: {
    id: "fleet.freshness.stale",
    defaultMessage: "Stale - showing last known values",
    description:
      "Freshness indicator when the server is serving a last-good value.",
  },
  gitopsNoPrs: {
    id: "fleet.map.bundle.gitopsNoPrs",
    defaultMessage: "No deployment changes in this range.",
    description:
      "Shown in the GitOps (deployment) group when the compare succeeded but found no pull requests.",
  },
  gitopsPrSummary: {
    id: "fleet.map.bundle.gitopsPrSummary",
    defaultMessage:
      "{count, plural, one {# PR} other {# PRs}} · deployment changes",
    description:
      "Count of GitOps (deployment) pull requests associated with a release bundle.",
  },
  gitopsUnavailable: {
    id: "fleet.map.bundle.gitopsUnavailable",
    defaultMessage: "Deployment changes are unavailable for this bundle.",
    description:
      "Shown in the GitOps (deployment) group when the compare range could not be determined or failed.",
  },
  historyAsOf: {
    id: "fleet.map.history.asOf",
    defaultMessage: "As of {time}",
    description:
      "Label above the population tiles showing the date/time of the history sample currently under the shared cursor.",
  },
  historyCursorGroup: {
    id: "fleet.map.history.cursorGroup",
    defaultMessage:
      "Population history over the last day. Use the left and right arrow keys to inspect past values, Home and End for the oldest and newest, and Escape to clear.",
    description:
      "Accessible name for the interactive population-tiles region that hosts the shared temporal cursor.",
  },
  historyCursorHint: {
    id: "fleet.map.history.cursorHint",
    defaultMessage: "Hover or use arrow keys to inspect history",
    description:
      "Subtle hint shown above the population tiles when the shared cursor is not engaged.",
  },
  historyReadout: {
    id: "fleet.map.history.readout",
    defaultMessage:
      "{time}: {sandboxes} sandboxes, {users} users, {logins} logins in the last 7 days",
    description:
      "Screen-reader announcement of the population values at the history sample under the shared cursor.",
  },
  infoDigest: {
    id: "fleet.map.info.digest",
    defaultMessage:
      "Content digest of the release bundle this instance is running - the exact value the identicon and identiname are derived from. One digest per instance; when instances in an environment run different digests, that environment is flagged as version drift.",
    description: "Help tooltip for the Digest field in the detail panel.",
  },
  infoDrift: {
    id: "fleet.map.info.drift",
    defaultMessage:
      "This instance runs a different active release bundle than its environment's hub, so the environment is internally inconsistent. Spokes should track their hub; a drift usually means a promotion did not reach this instance or it was pinned by hand.",
    description:
      "Help tooltip for the Version drift field in the detail panel.",
  },
  infoHealth: {
    id: "fleet.map.info.health",
    defaultMessage:
      "Argo CD health roll-up for this instance's workloads: Healthy, Progressing, or Degraded.",
    description: "Help tooltip for the Health field in the detail panel.",
  },
  infoIncoming: {
    id: "fleet.map.info.incoming",
    defaultMessage:
      "A newer bundle being promoted into this environment: the bundle in the open promotion PR, NOT yet deployed. It becomes the deployed release only once that PR merges and Argo CD syncs it.",
    description:
      "Help tooltip for the Proposed (incoming) release field in the detail panel.",
  },
  infoLogins: {
    id: "fleet.map.info.logins",
    defaultMessage:
      "Distinct users who signed in to this instance at least once in the last 7 days - a rolling measure of active usage, not the total account count.",
    description: "Help tooltip for the Logins tile in the detail panel.",
  },
  infoPromotion: {
    id: "fleet.map.info.promotion",
    defaultMessage:
      "Where this instance sits in the release flow: up-to-date (running the frontier bundle), promoting (a newer bundle is rolling out), or behind.",
    description: "Help tooltip for the Promotion field in the detail panel.",
  },
  infoRelease: {
    id: "fleet.map.info.release",
    defaultMessage:
      "The release-bundle version this instance is currently running. The chip is the bundle's identiname, a stable two-word alias so a bundle is easy to refer to instead of a long tag.",
    description: "Help tooltip for the Release field in the detail panel.",
  },
  infoRole: {
    id: "fleet.map.info.role",
    defaultMessage:
      "Whether this instance is a HUB (a management cluster that runs the control plane and drives promotion) or a SPOKE (a managed cluster whose workloads the hub reconciles).",
    description: "Help tooltip for the Role field in the detail panel.",
  },
  infoSync: {
    id: "fleet.map.info.sync",
    defaultMessage:
      "Argo CD sync state of this instance's Application. Synced = the live cluster matches the desired Git manifests; OutOfSync = it has drifted or a change is pending.",
    description: "Help tooltip for the Sync field in the detail panel.",
  },
  infoSyncAttempts: {
    id: "fleet.map.info.syncAttempts",
    defaultMessage:
      "How many times Argo CD has retried syncing this environment for the current promotion. A number that keeps climbing usually means the release-analysis gate is stuck - a newer commit superseded the one being checked - not that the tests are still running.",
    description:
      "Tooltip explaining the Sync attempts count and that a climbing value signals a wedged release-analysis gate rather than tests still running.",
  },
  infoUsers: {
    id: "fleet.map.info.users",
    defaultMessage:
      "Total registered user accounts for this instance, whether or not they have signed in recently.",
    description: "Help tooltip for the Users tile in the detail panel.",
  },
  legendFailed: {
    id: "fleet.map.legend.failed",
    defaultMessage: "Failed",
    description: "Gateway-donut legend row for failed gateways.",
  },
  legendOther: {
    id: "fleet.map.legend.other",
    defaultMessage: "Other",
    description:
      "Gateway-donut legend row for gateways in a phase outside running/provisioning/failed.",
  },
  legendProvisioning: {
    id: "fleet.map.legend.provisioning",
    defaultMessage: "Provisioning",
    description: "Gateway-donut legend row for provisioning gateways.",
  },
  legendRunning: {
    id: "fleet.map.legend.running",
    defaultMessage: "Running",
    description: "Gateway-donut legend row for running gateways.",
  },
  linkAnalysis: {
    id: "fleet.map.link.analysis",
    defaultMessage: "Analysis",
    description: "Link label for an instance's release analysis report.",
  },
  linkAnalysisDesc: {
    id: "fleet.map.link.analysis.desc",
    defaultMessage: "Release analysis report",
    description: "Sub-label describing the release-analysis link.",
  },
  linkArgo: {
    id: "fleet.map.link.argo",
    defaultMessage: "Argo CD",
    description: "Link label for an instance's Argo CD application.",
  },
  linkArgoDesc: {
    id: "fleet.map.link.argo.desc",
    defaultMessage: "Application sync and health",
    description: "Sub-label describing the Argo CD link.",
  },
  linkConsole: {
    id: "fleet.map.link.console",
    defaultMessage: "HyperShell Instance",
    description:
      "Link label for an instance's web console (its live front door).",
  },
  linkConsoleDesc: {
    id: "fleet.map.link.console.desc",
    defaultMessage: "Open the deployed web console",
    description: "Sub-label describing the HyperShell Instance link.",
  },
  linkGrafana: {
    id: "fleet.map.link.grafana",
    defaultMessage: "Grafana",
    description: "Link label for the cluster's own Grafana dashboards.",
  },
  linkGrafanaDesc: {
    id: "fleet.map.link.grafana.desc",
    defaultMessage: "Open observability dashboards",
    description: "Sub-label describing the Grafana link.",
  },
  linkPr: {
    id: "fleet.map.link.pr",
    defaultMessage: "Pull request",
    description: "Link label for an instance's open promotion pull request.",
  },
  linkPrDesc: {
    id: "fleet.map.link.pr.desc",
    defaultMessage: "Open promotion pull request",
    description: "Sub-label describing the promotion pull-request link.",
  },
  linksOperations: {
    id: "fleet.map.links.operations",
    defaultMessage: "Operations",
    description:
      "Group heading above the operational (Argo, PR, analysis) links, below the primary instance link.",
  },
  loadingPlane: {
    id: "fleet.plane.loading",
    defaultMessage: "Loading…",
    description: "Shown while a data source is loading.",
  },
  mapChange: {
    id: "fleet.map.change",
    defaultMessage: "change",
    description:
      "Label on the diamond at the head of the promotion spine, where a change enters the flow.",
  },
  mapFit: {
    id: "fleet.map.control.fit",
    defaultMessage: "Fit to view",
    description: "Accessible label for the map's fit-to-view control.",
  },
  mapHealthUnavailable: {
    id: "fleet.map.healthUnavailable",
    defaultMessage:
      "{count, plural, one {# cluster is not reporting health} other {# clusters are not reporting health}}",
    description:
      "Severe banner shown on the map only when one or more instances have no Argo health at all; the cluster may be unreachable.",
  },
  mapRegion: {
    id: "fleet.map.region",
    defaultMessage: "Promotion topology map",
    description: "Accessible label for the interactive map canvas region.",
  },
  mapZoomIn: {
    id: "fleet.map.control.zoomIn",
    defaultMessage: "Zoom in",
    description: "Accessible label for the map's zoom-in control.",
  },
  mapZoomOut: {
    id: "fleet.map.control.zoomOut",
    defaultMessage: "Zoom out",
    description: "Accessible label for the map's zoom-out control.",
  },
  metricTileAria: {
    id: "fleet.map.detail.metricTileAria",
    defaultMessage: "{label}: {value}",
    description:
      "Accessible label pairing a population metric tile's name with its value.",
  },
  moreInfo: {
    id: "fleet.map.info.more",
    defaultMessage: "More info",
    description: "Accessible label for a detail field's info tooltip button.",
  },
  pendingPromotion: {
    id: "fleet.promotion.pending",
    defaultMessage: "Promotion pending",
    description:
      "Marker shown when a newer release is proposed for an environment.",
  },
  prAuthoredBy: {
    id: "fleet.map.bundle.prAuthoredBy",
    defaultMessage: "by {author}",
    description: "Hover tooltip on a pull-request link naming its author.",
  },
  prEnvironments: {
    id: "fleet.map.bundle.prEnvironments",
    defaultMessage: "Deployed to {environments}",
    description:
      "Hover tooltip line attributing a GitOps deployment PR to the environments whose revision includes it.",
  },
  promoBehind: {
    id: "fleet.promotion.state.behind",
    defaultMessage: "Behind",
    description:
      "Promotion-state label when an instance is behind the frontier.",
  },
  promoPromoting: {
    id: "fleet.promotion.state.promoting",
    defaultMessage: "Promoting",
    description: "Promotion-state label when a newer bundle is rolling out.",
  },
  promoUpToDate: {
    id: "fleet.promotion.state.upToDate",
    defaultMessage: "Up to date",
    description:
      "Promotion-state label when an instance runs the frontier bundle.",
  },
  replicasRatioSeparator: {
    id: "fleet.agents.replicasRatioSeparator",
    defaultMessage: "/",
    description:
      "Separator between the active and total replica counts in the agent detail panel and per-cluster agents table.",
  },
  sdlcAge: {
    id: "fleet.sdlc.age",
    defaultMessage: "{days}d {hours}h",
    description:
      "Compact time-in-phase duration (days and hours) on an Agent SDLC card.",
  },
  sdlcAgeHm: {
    id: "fleet.sdlc.age.hm",
    defaultMessage: "{hours}h {minutes}m",
    description:
      "Compact time-in-phase duration (hours and minutes) for a card under a day old.",
  },
  sdlcAgeM: {
    id: "fleet.sdlc.age.m",
    defaultMessage: "{minutes}m",
    description:
      "Compact time-in-phase duration (minutes) for a card under an hour old.",
  },
  sdlcAgentDetailAria: {
    id: "fleet.sdlc.agent.detailAria",
    defaultMessage: "Open runtime details for the {agent} agent",
    description:
      "Accessible label for a row in the per-cluster agents table that opens runtime details.",
  },
  sdlcAxisNow: {
    id: "fleet.sdlc.axis.now",
    defaultMessage: "now",
    description: "Top label of the board's vertical time-in-phase axis.",
  },
  sdlcAxisTitle: {
    id: "fleet.sdlc.axis.title",
    defaultMessage: "Time in phase",
    description: "Caption for the board's vertical time-in-phase axis.",
  },
  sdlcBackToEscalations: {
    id: "fleet.sdlc.back.escalations",
    defaultMessage: "Back to escalations",
    description:
      "Link in the card details panel that returns to the escalations drawer.",
  },
  sdlcBoardEmpty: {
    id: "fleet.sdlc.board.empty",
    defaultMessage: "No work in progress.",
    description: "Shown when the Agent SDLC aging board has no cards.",
  },
  sdlcBundleIssueCount: {
    id: "fleet.sdlc.bundle.issueCount",
    defaultMessage: "{issues, plural, one {# issue} other {# issues}}",
    description:
      "Tracked-issue count part of a release bundle row's compact summary.",
  },
  sdlcBundleNoContents: {
    id: "fleet.sdlc.bundle.noContents",
    defaultMessage: "No pull requests in this bundle.",
    description:
      "Shown when an expanded release bundle has no pull requests at all (neither tracked issues nor one-off PRs), e.g. the oldest bundle with no prior build to diff against.",
  },
  sdlcBundleNoIssues: {
    id: "fleet.sdlc.bundle.noIssues",
    defaultMessage: "No tracked issues in this bundle.",
    description:
      "Shown when the Unreleased row has no in-flight tracked issues. The board tracks execution-record issues, so this reads empty when nothing shipped is awaiting a bundle.",
  },
  sdlcBundlePrCount: {
    id: "fleet.sdlc.bundle.prCount",
    defaultMessage: "{prs, plural, one {# PR} other {# PRs}}",
    description:
      "Pull-request count part of a release bundle row's compact summary.",
  },
  sdlcBundleSummary: {
    id: "fleet.sdlc.bundle.summary",
    defaultMessage: "{issues}, {prs}",
    description:
      "Compact summary in a release bundle row joining its tracked-issue count and its total pull-request count.",
  },
  sdlcCardOpen: {
    id: "fleet.sdlc.card.open",
    defaultMessage: "Open issue #{number}: {title}",
    description: "Accessible name for an Agent SDLC card link to its issue.",
  },
  sdlcClusterChip: {
    id: "fleet.sdlc.cluster.chip",
    defaultMessage: "+{count} more ({from}-{to})",
    description:
      "Label for a collapsed cluster of near-same-age cards in a board column.",
  },
  sdlcClusterTitle: {
    id: "fleet.sdlc.cluster.title",
    defaultMessage:
      "{count, plural, one {# item at a similar age} other {# items at a similar age}}",
    description: "Header of the popover listing a collapsed cluster's cards.",
  },
  sdlcColumnAria: {
    id: "fleet.sdlc.column.aria",
    defaultMessage:
      "{phase} column, {count, plural, one {# item} other {# items}}",
    description: "Accessible name for a phase column on the aging board.",
  },
  sdlcCurrentMarker: {
    id: "fleet.sdlc.matrix.currentMarker",
    defaultMessage: "current",
    description:
      "Non-color text marker in the matrix for the bundle a hub cluster currently runs.",
  },
  sdlcDetailBundle: {
    id: "fleet.sdlc.detail.bundle",
    defaultMessage: "Bundle",
    description:
      "Term label for the bundle row in the shipped-issue details panel.",
  },
  sdlcDetailMerged: {
    id: "fleet.sdlc.detail.merged",
    defaultMessage: "Merged",
    description:
      "Term label for the merged row in the shipped-issue details panel.",
  },
  sdlcDetailPhase: {
    id: "fleet.sdlc.detail.phase",
    defaultMessage: "Phase",
    description: "Term label for the phase row in the card details panel.",
  },
  sdlcDetailRework: {
    id: "fleet.sdlc.detail.rework",
    defaultMessage: "Rework",
    description: "Term label for the rework row in the card details panel.",
  },
  sdlcEscalationBlocked: {
    id: "fleet.sdlc.escalation.blocked",
    defaultMessage: "Blocked",
    description: "Escalation-lane bucket label for blocked work.",
  },
  sdlcEscalationDuplicate: {
    id: "fleet.sdlc.escalation.duplicate",
    defaultMessage: "Duplicate",
    description: "Escalation-lane bucket label for duplicate work.",
  },
  sdlcEscalationNeedsInput: {
    id: "fleet.sdlc.escalation.needsInput",
    defaultMessage: "Needs input",
    description: "Escalation-lane bucket label for work awaiting human input.",
  },
  sdlcEscalationPastSle: {
    id: "fleet.sdlc.escalation.pastSle",
    defaultMessage: "Past SLE",
    description:
      "Escalation-lane bucket label for work past its service-level expectation.",
  },
  sdlcFilterScope: {
    id: "fleet.sdlc.filter.scope",
    defaultMessage: "Scope",
    description: "Accessible label for the scope filter toggle group.",
  },
  sdlcHistoryEmpty: {
    id: "fleet.sdlc.history.empty",
    defaultMessage: "No label history.",
    description: "Shown when a card's label-event history is empty.",
  },
  sdlcHistoryTitle: {
    id: "fleet.sdlc.history.title",
    defaultMessage: "Label history",
    description: "Heading for a card's reconstructed label-event history.",
  },
  sdlcIssueLabel: {
    id: "fleet.sdlc.issue.label",
    defaultMessage: "#{number} {title}",
    description: "Issue reference combining its number and title.",
  },
  sdlcLaneCleared: {
    id: "fleet.sdlc.lane.cleared",
    defaultMessage: "Escalation lane clear - nothing blocked or stalled.",
    description: "Cleared state of the escalation lane (non-color cue).",
  },
  sdlcLaneTitle: {
    id: "fleet.sdlc.lane.title",
    defaultMessage: "Escalation lane",
    description: "Heading for the escalation lane.",
  },
  sdlcMatrixCurrent: {
    id: "fleet.sdlc.matrix.current",
    defaultMessage: "Current bundle on {cluster}",
    description: "Accessible label for a hub cluster's current-bundle marker.",
  },
  sdlcMatrixEmpty: {
    id: "fleet.sdlc.matrix.empty",
    defaultMessage: "No release history yet.",
    description: "Shown when the release-by-hub-cluster matrix has no rows.",
  },
  sdlcMatrixTitle: {
    id: "fleet.sdlc.matrix.title",
    defaultMessage: "Shipped",
    description:
      "Heading for the release-by-hub-cluster matrix (Shipped column).",
  },
  sdlcMergedAgo: {
    id: "fleet.sdlc.merged.ago",
    defaultMessage: "merged {days, plural, one {# day} other {# days}} ago",
    description: "Relative merge timestamp for a shipped issue.",
  },
  sdlcMergedBy: {
    id: "fleet.sdlc.merged.by",
    defaultMessage: "{ago} by {user}",
    description: "Merge timestamp with the user who merged the closing PR.",
  },
  sdlcMergedToday: {
    id: "fleet.sdlc.merged.today",
    defaultMessage: "merged today",
    description: "Relative merge timestamp for an issue merged today.",
  },
  sdlcNotificationsTitle: {
    id: "fleet.sdlc.notifications.title",
    defaultMessage: "Escalations",
    description:
      "Title of the escalations notification drawer and its bell trigger.",
  },
  sdlcOtherPRs: {
    id: "fleet.sdlc.matrix.otherPRs",
    defaultMessage: "Other pull requests",
    description:
      "Heading for the group of one-off pull requests in an expanded bundle row: PRs with no tracked execution-record issue.",
  },
  sdlcPhaseCode: {
    id: "fleet.sdlc.phase.code",
    defaultMessage: "Code",
    description: "Agent SDLC phase column: code implementation under review.",
  },
  sdlcPhaseCodeApproved: {
    id: "fleet.sdlc.phase.codeApproved",
    defaultMessage: "Code approved",
    description: "Agent SDLC phase column: code review approved.",
  },
  sdlcPhaseIntake: {
    id: "fleet.sdlc.phase.intake",
    defaultMessage: "Intake",
    description: "Agent SDLC phase column: newly admitted execution records.",
  },
  sdlcPhaseShipped: {
    id: "fleet.sdlc.phase.shipped",
    defaultMessage: "Shipped",
    description: "Agent SDLC terminal phase: merged and released.",
  },
  sdlcPhaseSpec: {
    id: "fleet.sdlc.phase.spec",
    defaultMessage: "Spec",
    description: "Agent SDLC phase column: specification under review.",
  },
  sdlcPhaseSpecApproved: {
    id: "fleet.sdlc.phase.specApproved",
    defaultMessage: "Spec approved",
    description: "Agent SDLC phase column: spec review approved.",
  },
  sdlcPhaseWorkable: {
    id: "fleet.sdlc.phase.workable",
    defaultMessage: "Workable",
    description: "Agent SDLC phase column: ready to start.",
  },
  sdlcReworkBlocked: {
    id: "fleet.sdlc.rework.blocked",
    defaultMessage: "This item is blocked.",
    description:
      "Non-color cue when an item has hit its rejection threshold and is blocked.",
  },
  sdlcReworkCount: {
    id: "fleet.sdlc.rework.count",
    defaultMessage: "x{count}",
    description:
      "Compact repeat-count badge on a collapsed reject loop in a card's label history.",
  },
  sdlcReworkLabel: {
    id: "fleet.sdlc.rework.label",
    defaultMessage: "{count} of {threshold} rejections",
    description: "Accessible label for the rework meter.",
  },
  sdlcReworkRemaining: {
    id: "fleet.sdlc.rework.remaining",
    defaultMessage:
      "{remaining, plural, one {# more rejection blocks this item.} other {# more rejections block this item.}}",
    description:
      "Non-color cue: how many more rejections until the item auto-blocks.",
  },
  sdlcReworkTooltip: {
    id: "fleet.sdlc.rework.tooltip",
    defaultMessage:
      "Rework: {count} of {threshold} review rejections in this phase. The item is auto-blocked at {threshold}.",
    description: "Tooltip explaining the rework meter and block threshold.",
  },
  sdlcScopeNoResults: {
    id: "fleet.sdlc.scope.noResults",
    defaultMessage: "No matching scopes",
    description:
      "Shown in the scope filter when the typeahead matches nothing.",
  },
  sdlcScopePlaceholder: {
    id: "fleet.sdlc.scope.placeholder",
    defaultMessage: "Filter by scope",
    description: "Placeholder for the scope typeahead multi-select.",
  },
  sdlcScopeSelected: {
    id: "fleet.sdlc.scope.selected",
    defaultMessage:
      "{count, plural, one {# scope selected} other {# scopes selected}}",
    description:
      "Scope typeahead placeholder summarising how many scopes are selected.",
  },
  sdlcSearchPlaceholder: {
    id: "fleet.sdlc.search.placeholder",
    defaultMessage: "Filter by number or title",
    description: "Placeholder for the Agent SDLC issue search field.",
  },
  sdlcSection: {
    id: "fleet.sdlc.section",
    defaultMessage: "Agent SDLC",
    description: "Card title for the Agent SDLC tab content.",
  },
  sdlcSleLine: {
    id: "fleet.sdlc.sle.line",
    defaultMessage: "Service-level expectation",
    description: "Accessible label for a column's dashed SLE line.",
  },
  sdlcSleShort: {
    id: "fleet.sdlc.sle.short",
    defaultMessage: "SLE",
    description:
      "Short pill label for the dashed service-level-expectation line.",
  },
  sdlcSleTooltip: {
    id: "fleet.sdlc.sle.tooltip",
    defaultMessage:
      "Service-level expectation: {duration}. The target time an item spends in this phase; items below the line are past SLE and escalate.",
    description:
      "Tooltip explaining the SLE line, including the target duration.",
  },
  sdlcTabAgent: {
    id: "fleet.sdlc.tab.agent",
    defaultMessage: "Agent SDLC",
    description: "Tab label for the Agent SDLC view.",
  },
  sdlcTabFleetMap: {
    id: "fleet.sdlc.tab.fleetMap",
    defaultMessage: "Fleet Map",
    description: "Tab label for the existing Fleet Map view.",
  },
  sdlcTabs: {
    id: "fleet.sdlc.tabs.aria",
    defaultMessage: "Dashboard views",
    description: "Accessible name for the Fleet Map / Agent SDLC tab control.",
  },
  sdlcTimeInPhase: {
    id: "fleet.sdlc.timeInPhase",
    defaultMessage: "In phase {duration}",
    description: "Accessible label for a card's time-in-phase.",
  },
  sdlcTrackedIssues: {
    id: "fleet.sdlc.matrix.trackedIssues",
    defaultMessage: "Tracked issues",
    description:
      "Heading for the group of tracked execution-record issues in an expanded bundle row: the bundle's PRs grouped under the issue they resolve to.",
  },
  sdlcUnreleased: {
    id: "fleet.sdlc.matrix.unreleased",
    defaultMessage: "Unreleased",
    description: "Matrix row for merged work not yet in a released bundle.",
  },
  sectionCapabilities: {
    id: "fleet.section.capabilities",
    defaultMessage: "Capabilities",
    description:
      "Heading for the cluster's self-declared capability labels in the detail panel.",
  },
  sectionDeployedOn: {
    id: "fleet.section.deployedOn",
    defaultMessage: "Deployed on",
    description:
      "Heading above the list of instances running a release bundle.",
  },
  sectionGateways: {
    id: "fleet.section.gateways",
    defaultMessage: "Gateways",
    description:
      "Heading above the gateway donut + phase breakdown in an instance's detail panel.",
  },
  sectionGitopsChanges: {
    id: "fleet.section.gitopsChanges",
    defaultMessage: "Deployment changes (GitOps)",
    description:
      "Heading above the GitOps delivery-repository deployment pull-request group in a release bundle's details.",
  },
  sectionInBundle: {
    id: "fleet.section.inBundle",
    defaultMessage: "In this bundle",
    description: "Heading above the pull-request list in a release bundle.",
  },
  sectionInstances: {
    id: "fleet.section.instances",
    defaultMessage: "Instances",
    description: "Heading for the managed instances section.",
  },
  sectionLogins: {
    id: "fleet.section.logins",
    defaultMessage: "Logins 7d",
    description:
      "Label for the 7-day unique-login metric tile in the detail panel.",
  },
  sectionManagedClusters: {
    id: "fleet.section.managedClusters",
    defaultMessage: "Managed clusters",
    description:
      "Heading for the per-managed-cluster gateway/sandbox breakdown in the detail panel.",
  },
  sectionMap: {
    id: "fleet.section.map",
    defaultMessage: "Promotion topology",
    description: "Heading for the interactive promotion topology map section.",
  },
  sectionPromotion: {
    id: "fleet.section.promotion",
    defaultMessage: "Promotion path",
    description: "Heading for the promotion path section.",
  },
  sectionReleases: {
    id: "fleet.section.releases",
    defaultMessage: "Release bundles",
    description: "Accessible label for the release freight bar.",
  },
  sectionSandboxes: {
    id: "fleet.section.sandboxes",
    defaultMessage: "Sandboxes",
    description:
      "Label for the active-sandbox metric tile in the detail panel.",
  },
  sectionUsers: {
    id: "fleet.section.users",
    defaultMessage: "Users",
    description:
      "Label for the registered-user metric tile in the detail panel.",
  },
  sessionExpiredBody: {
    id: "fleet.session.expired.body",
    defaultMessage: "Your session has expired. Sign in again to continue.",
    description:
      "Body of the full-page takeover shown when the BFF returns 401 for API calls.",
  },
  sessionExpiredTitle: {
    id: "fleet.session.expired.title",
    defaultMessage: "Session expired",
    description:
      "Title of the full-page takeover shown when the user's session is stale.",
  },
  sessionSignIn: {
    id: "fleet.session.signIn",
    defaultMessage: "Sign in again",
    description:
      "Button that reloads the document to re-run the oauth-proxy sign-in flow.",
  },
  spokeAttributionUnavailable: {
    id: "fleet.spoke.attributionUnavailable",
    defaultMessage:
      "Cluster topology is unavailable, so these managed clusters could not be attributed to a hub.",
    description:
      "Note shown above an unattributed, flat list of managed clusters when topology data is missing.",
  },
  spokeGatewaysCount: {
    id: "fleet.spoke.gatewaysCount",
    defaultMessage: "{count, plural, one {# gateway} other {# gateways}}",
    description:
      "Gateway total shown to the right of a managed-cluster row in the detail panel.",
  },
  spokeRemote: {
    id: "fleet.spoke.remote",
    defaultMessage: "Remote",
    description:
      "Compact badge marking a managed cluster that runs remote from its hub.",
  },
  spokeRemoteGroup: {
    id: "fleet.spoke.remoteGroup",
    defaultMessage: "Remote",
    description:
      "Sub-heading for the group of managed clusters that run remote from the hub.",
  },
  spokeSandboxesCount: {
    id: "fleet.spoke.sandboxesCount",
    defaultMessage: "{count, plural, one {# sandbox} other {# sandboxes}}",
    description:
      "Sandbox total shown to the right of a managed-cluster row in the detail panel.",
  },
  spokeUnattributedGroup: {
    id: "fleet.spoke.unattributedGroup",
    defaultMessage: "Unattributed",
    description:
      "Sub-heading for managed clusters present in the metrics but absent from the hub's topology.",
  },
  statusDegraded: {
    id: "fleet.status.degraded",
    defaultMessage: "Degraded",
    description: "Status label for a degraded or missing resource.",
  },
  statusFailed: {
    id: "fleet.status.failed",
    defaultMessage: "Failed",
    description: "Status label for a failed gate.",
  },
  statusHealthy: {
    id: "fleet.status.healthy",
    defaultMessage: "Healthy",
    description: "Status label for a healthy resource.",
  },
  statusOutOfSync: {
    id: "fleet.status.outOfSync",
    defaultMessage: "Out of sync",
    description: "Status label for an out-of-sync resource.",
  },
  statusPassed: {
    id: "fleet.status.passed",
    defaultMessage: "Passed",
    description: "Status label for a passing gate.",
  },
  statusPending: {
    id: "fleet.status.pending",
    defaultMessage: "Pending",
    description: "Status label for a pending or running gate.",
  },
  statusProgressing: {
    id: "fleet.status.progressing",
    defaultMessage: "Progressing",
    description: "Status label for a progressing resource.",
  },
  statusSuspended: {
    id: "fleet.status.suspended",
    defaultMessage: "Suspended",
    description: "Status label for a suspended resource.",
  },
  statusSynced: {
    id: "fleet.status.synced",
    defaultMessage: "Synced",
    description: "Status label for a synced resource.",
  },
  statusUnknown: {
    id: "fleet.status.unknown",
    defaultMessage: "Unknown",
    description: "Status label for an unknown or unrecognized state.",
  },
  tabAgents: {
    id: "fleet.map.tab.agents",
    defaultMessage: "Agents",
    description:
      "Detail-panel tab showing the SDLC agents running on the selected cluster.",
  },
  tabBundle: {
    id: "fleet.map.tab.bundle",
    defaultMessage: "Bundle",
    description:
      "Detail-panel tab showing the selected instance's release bundle.",
  },
  tabDetails: {
    id: "fleet.map.tab.details",
    defaultMessage: "Details",
    description:
      "Detail-panel tab showing the selected instance's full record.",
  },
  tabLinks: {
    id: "fleet.map.tab.links",
    defaultMessage: "Links",
    description: "Detail-panel tab showing the selected instance's deep links.",
  },
  valueNone: {
    id: "fleet.value.none",
    defaultMessage: "-",
    description: "Placeholder shown for a missing value.",
  },
});
