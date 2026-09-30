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
  loadingPlane: {
    id: "fleet.plane.loading",
    defaultMessage: "Loading…",
    description: "Shown while a data source is loading.",
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
  sectionPromotion: {
    id: "fleet.section.promotion",
    defaultMessage: "Promotion path",
    description: "Heading for the promotion path section.",
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
