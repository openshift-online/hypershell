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
  detailProposed: {
    id: "fleet.map.detail.proposed",
    defaultMessage: "Proposed release",
    description: "Detail label for the release proposed for an instance.",
  },
  detailUsers: {
    id: "fleet.map.detail.users",
    defaultMessage: "Users",
    description: "Detail label for the number of users on an instance.",
  },
  drawerClose: {
    id: "fleet.map.drawer.close",
    defaultMessage: "Close details",
    description: "Accessible label for the detail panel's close button.",
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
  linkAnalysis: {
    id: "fleet.map.link.analysis",
    defaultMessage: "Analysis",
    description: "Link label for an instance's release analysis report.",
  },
  linkArgo: {
    id: "fleet.map.link.argo",
    defaultMessage: "Argo CD",
    description: "Link label for an instance's Argo CD application.",
  },
  linkConsole: {
    id: "fleet.map.link.console",
    defaultMessage: "Console",
    description: "Link label for an instance's web console.",
  },
  linkPr: {
    id: "fleet.map.link.pr",
    defaultMessage: "Pull request",
    description: "Link label for an instance's open promotion pull request.",
  },
  loadingPlane: {
    id: "fleet.plane.loading",
    defaultMessage: "Loading…",
    description: "Shown while a data source is loading.",
  },
  mapFit: {
    id: "fleet.map.control.fit",
    defaultMessage: "Fit to view",
    description: "Accessible label for the map's fit-to-view control.",
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
  pendingPromotion: {
    id: "fleet.promotion.pending",
    defaultMessage: "Promotion pending",
    description:
      "Marker shown when a newer release is proposed for an environment.",
  },
  sectionInstances: {
    id: "fleet.section.instances",
    defaultMessage: "Instances",
    description: "Heading for the managed instances section.",
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
  valueNone: {
    id: "fleet.value.none",
    defaultMessage: "-",
    description: "Placeholder shown for a missing value.",
  },
});
