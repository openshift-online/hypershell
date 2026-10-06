// Central message catalog. Keys are sorted (enforced by ESLint) and every entry
// carries an explicit id + defaultMessage + description
// (formatjs enforce-id/enforce-default-message/enforce-description).
// UI copy carries NO fleet identity - instance/environment names are data, not strings.

import { defineMessages } from "react-intl";

export const messages = defineMessages({
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
