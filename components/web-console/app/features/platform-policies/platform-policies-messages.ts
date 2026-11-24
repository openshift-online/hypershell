import { defineMessages } from "react-intl";

/** Copy for the platform policy list, toggle, and detail views (WEB-PADM). */
export const platformPoliciesMessages = defineMessages({
  pageTitle: {
    id: "app.platformPolicies.page.title",
    defaultMessage: "Platform policies",
    description: "Browser title and heading of the platform policies page.",
  },
  pageDescription: {
    id: "app.platformPolicies.page.description",
    defaultMessage:
      "Set sandbox rules for many gateways. Gateway users see these rules and their audit findings on each gateway.",
    description: "Description under the platform policies heading.",
  },
  detailPageTitle: {
    id: "app.platformPolicies.page.detailTitle",
    defaultMessage: "Platform policy",
    description: "Browser title of the platform policy detail page.",
  },
  detailPageDescription: {
    id: "app.platformPolicies.page.detailDescription",
    defaultMessage:
      "Review a platform policy's rules, rollout, and audit findings.",
    description: "Browser metadata description of the policy detail page.",
  },
  navAdministration: {
    id: "app.platformPolicies.nav.administration",
    defaultMessage: "Administration",
    description: "Navigation group for platform administration pages.",
  },
  navPlatformPolicies: {
    id: "app.platformPolicies.nav.platformPolicies",
    defaultMessage: "Platform policies",
    description: "Navigation entry and breadcrumb for platform policies.",
  },
  breadcrumbCreate: {
    id: "app.platformPolicies.breadcrumb.create",
    defaultMessage: "Create platform policy",
    description: "Breadcrumb for the create platform policy page.",
  },
  breadcrumbEdit: {
    id: "app.platformPolicies.breadcrumb.edit",
    defaultMessage: "Edit",
    description: "Breadcrumb for the edit platform policy page.",
  },
  breadcrumbPolicy: {
    id: "app.platformPolicies.breadcrumb.policy",
    defaultMessage: "Policy",
    description: "Breadcrumb fallback before the policy name loads.",
  },
  createPolicy: {
    id: "app.platformPolicies.createPolicy",
    defaultMessage: "Create platform policy",
    description: "Button that opens the create platform policy page.",
  },
  loading: {
    id: "app.platformPolicies.loading",
    defaultMessage: "Loading platform policies",
    description: "Accessible label while platform policies load.",
  },
  loadErrorTitle: {
    id: "app.platformPolicies.loadErrorTitle",
    defaultMessage: "Could not load platform policies",
    description: "Title when platform policies could not be loaded.",
  },
  loadErrorBody: {
    id: "app.platformPolicies.loadErrorBody",
    defaultMessage: "Check your connection and try again.",
    description: "Body when platform policies could not be loaded.",
  },
  retry: {
    id: "app.platformPolicies.retry",
    defaultMessage: "Try again",
    description: "Button that reloads platform policies.",
  },
  unsupportedAlertTitle: {
    id: "app.platformPolicies.unsupportedAlertTitle",
    defaultMessage:
      "{name} cannot be enforced on {count, plural, one {# gateway} other {# gateways}}",
    description:
      "Warning title when a policy cannot be enforced on some gateways.",
  },
  unsupportedAlertBody: {
    id: "app.platformPolicies.unsupportedAlertBody",
    defaultMessage: "{gateway}: {message}",
    description:
      "Warning body naming an unsupported gateway and the reason it cannot enforce.",
  },
  unsupportedAlertBodyNoReason: {
    id: "app.platformPolicies.unsupportedAlertBodyNoReason",
    defaultMessage:
      "{gateway} reported that it cannot enforce this policy's rules.",
    description: "Warning body when an unsupported gateway reported no reason.",
  },
  reviewRollout: {
    id: "app.platformPolicies.reviewRollout",
    defaultMessage: "Review rollout",
    description: "Link from the unsupported warning to the policy gateways.",
  },
  enforcementDescriptionEnforce: {
    id: "app.platformPolicies.enforcement.descriptionEnforce",
    defaultMessage:
      "Every platform and gateway policy is enforced on every gateway.",
    description: "Platform enforcement card text when the mode is Enforced.",
  },
  enforcementDescriptionAudit: {
    id: "app.platformPolicies.enforcement.descriptionAudit",
    defaultMessage:
      "Audit only is the minimum. A policy rejects operations only when its own setting is Enforced.",
    description: "Platform enforcement card text when the mode is Audit only.",
  },
  enforcedByPolicySetting: {
    id: "app.platformPolicies.enforcement.enforcedByPolicySetting",
    defaultMessage: "Enforced by policy setting",
    description: "Term counting policies enforced by their own setting.",
  },
  policiesOfTotal: {
    id: "app.platformPolicies.enforcement.policiesOfTotal",
    defaultMessage:
      "{count} of {total, plural, one {# policy} other {# policies}}",
    description: "Count of policies out of all policies.",
  },
  switchToEnforced: {
    id: "app.platformPolicies.enforcement.switchToEnforced",
    defaultMessage: "Switch to Enforced...",
    description: "Opens the confirmation to enforce every policy.",
  },
  switchToAudit: {
    id: "app.platformPolicies.enforcement.switchToAudit",
    defaultMessage: "Switch to Audit only...",
    description: "Opens the confirmation to return to audit only.",
  },
  findingsTitle: {
    id: "app.platformPolicies.findings.title",
    defaultMessage: "Audit findings, last 24 hours",
    description: "Heading of the audit findings summary card.",
  },
  findingsWouldBeRejected: {
    id: "app.platformPolicies.findings.wouldBeRejected",
    defaultMessage: "{count} would be rejected",
    description: "Status next to the findings heading.",
  },
  findingsNothing: {
    id: "app.platformPolicies.findings.nothing",
    defaultMessage: "No audit-only policies. Nothing to report.",
    description: "Findings card text when no policy is audit only.",
  },
  findingsWouldReject: {
    id: "app.platformPolicies.findings.wouldReject",
    defaultMessage: "Would reject",
    description: "Term for operations that would have been rejected.",
  },
  findingsWouldAddDefaults: {
    id: "app.platformPolicies.findings.wouldAddDefaults",
    defaultMessage: "Would add defaults",
    description: "Term for operations that would have received defaults.",
  },
  findingsGatewaysAffected: {
    id: "app.platformPolicies.findings.gatewaysAffected",
    defaultMessage: "Gateways affected",
    description: "Term for gateways with would-reject findings.",
  },
  operationsCount: {
    id: "app.platformPolicies.findings.operationsCount",
    defaultMessage: "{count, plural, one {# operation} other {# operations}}",
    description: "A number of operations.",
  },
  countOfTotal: {
    id: "app.platformPolicies.findings.countOfTotal",
    defaultMessage: "{count} of {total}",
    description: "A count out of a total.",
  },
  policiesTitle: {
    id: "app.platformPolicies.policiesTitle",
    defaultMessage: "Policies",
    description: "Heading of the platform policies table card.",
  },
  filterLabel: {
    id: "app.platformPolicies.filterLabel",
    defaultMessage: "Filter policies by name",
    description: "Accessible name of the policy name filter.",
  },
  filterPlaceholder: {
    id: "app.platformPolicies.filterPlaceholder",
    defaultMessage: "Filter by name",
    description: "Placeholder of the policy name filter.",
  },
  tableLabel: {
    id: "app.platformPolicies.tableLabel",
    defaultMessage: "Platform policies",
    description: "Accessible name of the platform policies table.",
  },
  columnPolicy: {
    id: "app.platformPolicies.column.policy",
    defaultMessage: "Policy",
    description: "Column heading for the policy name.",
  },
  columnAppliesTo: {
    id: "app.platformPolicies.column.appliesTo",
    defaultMessage: "Applies to",
    description: "Column heading for the policy target.",
  },
  columnRules: {
    id: "app.platformPolicies.column.rules",
    defaultMessage: "Rules",
    description: "Column heading for the resources a policy bounds.",
  },
  columnEnforcement: {
    id: "app.platformPolicies.column.enforcement",
    defaultMessage: "Enforcement",
    description: "Column heading for the effective mode.",
  },
  columnApplied: {
    id: "app.platformPolicies.column.applied",
    defaultMessage: "Applied",
    description: "Column heading for the rollout status.",
  },
  columnWouldReject: {
    id: "app.platformPolicies.column.wouldReject",
    defaultMessage: "Would reject, last 24 hours",
    description: "Column heading for would-reject findings in 24 hours.",
  },
  columnActions: {
    id: "app.platformPolicies.column.actions",
    defaultMessage: "Actions",
    description: "Accessible heading for row actions.",
  },
  notApplicable: {
    id: "app.platformPolicies.notApplicable",
    defaultMessage: "-",
    description: "Placeholder when a value does not apply.",
  },
  appliesTo: {
    id: "app.platformPolicies.appliesTo",
    defaultMessage:
      "{kind, select, all {All gateways} except {All gateways except {count}} other {{count, plural, one {# gateway} other {# gateways}}}}",
    description: "Summary of the gateways a policy applies to.",
  },
  appliesToSentence: {
    id: "app.platformPolicies.appliesToSentence",
    defaultMessage:
      "Platform policy. Applies to {kind, select, all {all gateways} except {all gateways except {count}} other {{count, plural, one {# gateway} other {# gateways}}}}.",
    description: "Description under the policy name on the detail page.",
  },
  appliedOn: {
    id: "app.platformPolicies.appliedOn",
    defaultMessage:
      "Applied on {applied} of {total, plural, one {# gateway} other {# gateways}}",
    description: "Rollout status of a policy.",
  },
  cannotEnforceCount: {
    id: "app.platformPolicies.cannotEnforceCount",
    defaultMessage: "{count} cannot enforce",
    description: "Number of gateways that cannot enforce a policy.",
  },
  pendingCount: {
    id: "app.platformPolicies.pendingCount",
    defaultMessage: "{count} pending",
    description: "Number of gateways with a pending rollout.",
  },
  startEnforcing: {
    id: "app.platformPolicies.action.startEnforcing",
    defaultMessage: "Start enforcing...",
    description: "Action that opens the confirmation to enforce a policy.",
  },
  returnToAudit: {
    id: "app.platformPolicies.action.returnToAudit",
    defaultMessage: "Return to audit only...",
    description: "Action that opens the confirmation to audit a policy.",
  },
  startEnforcingDisabled: {
    id: "app.platformPolicies.action.startEnforcingDisabled",
    defaultMessage: "Platform enforcement already enforces every policy.",
    description: "Why Start enforcing is unavailable.",
  },
  returnToAuditDisabled: {
    id: "app.platformPolicies.action.returnToAuditDisabled",
    defaultMessage: "Platform enforcement is Enforced for every policy.",
    description: "Why Return to audit only is unavailable.",
  },
  editPolicy: {
    id: "app.platformPolicies.action.editPolicy",
    defaultMessage: "Edit policy",
    description: "Row action that opens the policy editor.",
  },
  edit: {
    id: "app.platformPolicies.action.edit",
    defaultMessage: "Edit",
    description: "Detail page action that opens the policy editor.",
  },
  deletePolicy: {
    id: "app.platformPolicies.action.deletePolicy",
    defaultMessage: "Delete policy",
    description: "Action that deletes a policy.",
  },
  delete: {
    id: "app.platformPolicies.action.delete",
    defaultMessage: "Delete",
    description: "Detail page action that deletes a policy.",
  },
  builtInCannotDelete: {
    id: "app.platformPolicies.action.builtInCannotDelete",
    defaultMessage: "Built-in policies cannot be deleted.",
    description: "Why delete is unavailable for a built-in policy.",
  },
  emptyTitle: {
    id: "app.platformPolicies.empty.title",
    defaultMessage: "No platform policies",
    description: "Empty state title when no policy exists.",
  },
  emptyBody: {
    id: "app.platformPolicies.empty.body",
    defaultMessage:
      "Create a policy to set sandbox rules on your gateways. New policies start in audit only, so you can review findings before anything is rejected.",
    description: "Empty state body when no policy exists.",
  },
  noMatchesTitle: {
    id: "app.platformPolicies.noMatches.title",
    defaultMessage: "No policies match this filter",
    description: "Empty state when the name filter matches nothing.",
  },
  clearFilter: {
    id: "app.platformPolicies.noMatches.clear",
    defaultMessage: "Clear filter",
    description: "Clears the policy name filter.",
  },
  dialogEnforceAllTitle: {
    id: "app.platformPolicies.dialog.enforceAllTitle",
    defaultMessage:
      "Enforce every policy on all {count, plural, one {# gateway} other {# gateways}}?",
    description: "Confirmation title when switching platform enforcement on.",
  },
  dialogEnforceAllBody: {
    id: "app.platformPolicies.dialog.enforceAllBody",
    defaultMessage:
      "OpenShell will reject operations that break any platform or gateway policy and add missing defaults.",
    description: "Confirmation body when switching platform enforcement on.",
  },
  dialogEnforceAllConfirm: {
    id: "app.platformPolicies.dialog.enforceAllConfirm",
    defaultMessage:
      "Enforce on {count, plural, one {# gateway} other {# gateways}}",
    description: "Confirm button when switching platform enforcement on.",
  },
  dialogStartEnforcingTitle: {
    id: "app.platformPolicies.dialog.startEnforcingTitle",
    defaultMessage: "Start enforcing {name}?",
    description: "Confirmation title when enforcing one policy.",
  },
  dialogStartEnforcingBody: {
    id: "app.platformPolicies.dialog.startEnforcingBody",
    defaultMessage:
      "OpenShell will reject operations that break this policy on every gateway it applies to, and add its defaults.",
    description: "Confirmation body when enforcing one policy.",
  },
  dialogStartEnforcingConfirm: {
    id: "app.platformPolicies.dialog.startEnforcingConfirm",
    defaultMessage: "Start enforcing",
    description: "Confirm button when enforcing one policy.",
  },
  dialogImpactTitle: {
    id: "app.platformPolicies.dialog.impactTitle",
    defaultMessage:
      "In the last 24 hours, {count, plural, one {# operation} other {# operations}} would have been rejected",
    description: "Impact alert title in an enforce confirmation.",
  },
  dialogImpactBody: {
    id: "app.platformPolicies.dialog.impactBody",
    defaultMessage:
      "{count, plural, one {# operation} other {# operations}} would have received default values.",
    description: "Impact alert body in an enforce confirmation.",
  },
  dialogImpactTable: {
    id: "app.platformPolicies.dialog.impactTable",
    defaultMessage: "Impact by gateway",
    description: "Accessible name of the per-gateway impact table.",
  },
  dialogImpactNone: {
    id: "app.platformPolicies.dialog.impactNone",
    defaultMessage:
      "No operations in the last 24 hours would have been rejected on these gateways.",
    description: "Text when no gateway has would-reject findings.",
  },
  columnGateway: {
    id: "app.platformPolicies.column.gateway",
    defaultMessage: "Gateway",
    description: "Column heading for a gateway.",
  },
  columnNote: {
    id: "app.platformPolicies.column.note",
    defaultMessage: "Note",
    description: "Column heading for a note about a gateway.",
  },
  columnWouldRejectShort: {
    id: "app.platformPolicies.column.wouldRejectShort",
    defaultMessage: "Would reject",
    description: "Column heading for would-reject counts.",
  },
  noteRevisionPending: {
    id: "app.platformPolicies.note.revisionPending",
    defaultMessage: "Revision pending",
    description: "Note when a gateway has not applied the latest revision.",
  },
  noteCannotEnforce: {
    id: "app.platformPolicies.note.cannotEnforce",
    defaultMessage: "Cannot enforce. {message}",
    description: "Note when a gateway cannot enforce, with its reason.",
  },
  noteCannotEnforceNoReason: {
    id: "app.platformPolicies.note.cannotEnforceNoReason",
    defaultMessage: "Cannot enforce.",
    description: "Note when a gateway cannot enforce, without a reason.",
  },
  dialogAuditAllTitle: {
    id: "app.platformPolicies.dialog.auditAllTitle",
    defaultMessage: "Switch platform enforcement to Audit only?",
    description: "Confirmation title when returning to audit only.",
  },
  dialogAuditAllBody: {
    id: "app.platformPolicies.dialog.auditAllBody",
    defaultMessage:
      "Policies that are not enforced by their own setting stop rejecting operations on {count, plural, one {# gateway} other {# gateways}}. HyperShell records them as audit findings instead.",
    description: "Confirmation body when returning to audit only.",
  },
  dialogAuditAllConfirm: {
    id: "app.platformPolicies.dialog.auditAllConfirm",
    defaultMessage: "Switch to Audit only",
    description: "Confirm button when returning to audit only.",
  },
  dialogAuditPolicyTitle: {
    id: "app.platformPolicies.dialog.auditPolicyTitle",
    defaultMessage: "Return {name} to audit only?",
    description: "Confirmation title when one policy returns to audit only.",
  },
  dialogAuditPolicyBody: {
    id: "app.platformPolicies.dialog.auditPolicyBody",
    defaultMessage:
      "This policy stops rejecting operations on {count, plural, one {# gateway} other {# gateways}}. HyperShell records them as audit findings instead.",
    description: "Confirmation body when one policy returns to audit only.",
  },
  dialogAuditPolicyConfirm: {
    id: "app.platformPolicies.dialog.auditPolicyConfirm",
    defaultMessage: "Return to audit only",
    description: "Confirm button when one policy returns to audit only.",
  },
  dialogDeleteTitle: {
    id: "app.platformPolicies.dialog.deleteTitle",
    defaultMessage: "Delete {name}?",
    description: "Confirmation title when deleting a policy.",
  },
  dialogDeleteBody: {
    id: "app.platformPolicies.dialog.deleteBody",
    defaultMessage:
      "The policy is removed from {count, plural, one {# gateway} other {# gateways}}. This action cannot be undone.",
    description: "Confirmation body when deleting a policy.",
  },
  cancel: {
    id: "app.platformPolicies.dialog.cancel",
    defaultMessage: "Cancel",
    description: "Closes a confirmation without changes.",
  },
  writeDeniedTitle: {
    id: "app.platformPolicies.write.deniedTitle",
    defaultMessage: "You are not allowed to make this change",
    description:
      "Alert title when the API refuses a policy write with 403 (not retried).",
  },
  writeDeniedBody: {
    id: "app.platformPolicies.write.deniedBody",
    defaultMessage:
      "Your access may have changed. Ask a platform administrator for the platform-policy:admin role.",
    description: "Alert body when a policy write is denied.",
  },
  writeConflictTitle: {
    id: "app.platformPolicies.write.conflictTitle",
    defaultMessage: "This change conflicts with the current policy",
    description: "Alert title when the API returns 409 for a policy write.",
  },
  writeConflictBody: {
    id: "app.platformPolicies.write.conflictBody",
    defaultMessage:
      "Someone else changed it, or the policy is built in. Review the latest values and try again.",
    description: "Alert body when a policy write conflicts.",
  },
  writeFailedTitle: {
    id: "app.platformPolicies.write.failedTitle",
    defaultMessage: "The change was not saved",
    description: "Alert title when a policy write fails for another reason.",
  },
  writeFailedBody: {
    id: "app.platformPolicies.write.failedBody",
    defaultMessage: "Try again in a moment.",
    description: "Alert body when a policy write fails for another reason.",
  },
  dismiss: {
    id: "app.platformPolicies.write.dismiss",
    defaultMessage: "Close",
    description: "Accessible name of the alert close button.",
  },
  detailTabsLabel: {
    id: "app.platformPolicies.detail.tabsLabel",
    defaultMessage: "Policy overview, gateways, and audit findings",
    description: "Accessible name of the policy detail tabs.",
  },
  tabOverview: {
    id: "app.platformPolicies.detail.tabOverview",
    defaultMessage: "Overview",
    description: "Policy detail tab with rules and message.",
  },
  tabGateways: {
    id: "app.platformPolicies.detail.tabGateways",
    defaultMessage: "Gateways",
    description: "Policy detail tab with per-gateway rollout.",
  },
  tabFindings: {
    id: "app.platformPolicies.detail.tabFindings",
    defaultMessage: "Audit findings ({count})",
    description: "Policy detail tab with audit findings and their count.",
  },
  rulesTitle: {
    id: "app.platformPolicies.detail.rulesTitle",
    defaultMessage: "Rules",
    description: "Heading of the policy rules.",
  },
  noRecognizedRules: {
    id: "app.platformPolicies.detail.noRecognizedRules",
    defaultMessage: "This policy has no resource rules.",
    description: "Text when a policy has no plain-words resource rules.",
  },
  cedarSourceTitle: {
    id: "app.platformPolicies.detail.cedarSourceTitle",
    defaultMessage: "Cedar source",
    description: "Toggle text for the full Cedar source of a policy.",
  },
  enforcementValue: {
    id: "app.platformPolicies.detail.enforcementValue",
    defaultMessage:
      "{mode, select, enforce {Enforced} other {Audit only}} ({reason, select, platform {platform enforcement} other {policy setting}}).",
    description: "The effective mode and its reason on the detail page.",
  },
  lastUpdated: {
    id: "app.platformPolicies.detail.lastUpdated",
    defaultMessage: "Last updated",
    description: "Term for when the policy last changed.",
  },
  gatewaysTableLabel: {
    id: "app.platformPolicies.detail.gatewaysTable",
    defaultMessage: "Gateways this policy applies to",
    description: "Accessible name of the per-gateway rollout table.",
  },
  columnRevision: {
    id: "app.platformPolicies.column.revision",
    defaultMessage: "Revision",
    description: "Column heading for the bundle revision.",
  },
  columnDetail: {
    id: "app.platformPolicies.column.detail",
    defaultMessage: "Detail",
    description: "Column heading for rollout or finding detail.",
  },
  columnTime: {
    id: "app.platformPolicies.column.time",
    defaultMessage: "Time",
    description: "Column heading for when a finding occurred.",
  },
  columnUser: {
    id: "app.platformPolicies.column.user",
    defaultMessage: "User",
    description: "Column heading for who made the operation.",
  },
  columnOutcome: {
    id: "app.platformPolicies.column.outcome",
    defaultMessage: "Outcome",
    description: "Column heading for the finding outcome.",
  },
  rolloutApplied: {
    id: "app.platformPolicies.rollout.applied",
    defaultMessage: "Applied",
    description: "Rollout condition when the gateway applied the policy.",
  },
  rolloutPending: {
    id: "app.platformPolicies.rollout.pending",
    defaultMessage: "Pending",
    description: "Rollout condition while the gateway has not applied it.",
  },
  rolloutUnsupported: {
    id: "app.platformPolicies.rollout.unsupported",
    defaultMessage: "Cannot enforce",
    description: "Rollout condition when the gateway cannot enforce it.",
  },
  noGateways: {
    id: "app.platformPolicies.detail.noGateways",
    defaultMessage: "This policy does not apply to any gateway yet.",
    description: "Empty text of the per-gateway rollout table.",
  },
  findingsTableLabel: {
    id: "app.platformPolicies.detail.findingsTable",
    defaultMessage: "Audit findings for this policy",
    description: "Accessible name of the policy findings table.",
  },
  findingsGatewayFilter: {
    id: "app.platformPolicies.detail.findingsGatewayFilter",
    defaultMessage: "Gateway",
    description: "Label of the findings gateway filter.",
  },
  findingsAllGateways: {
    id: "app.platformPolicies.detail.findingsAllGateways",
    defaultMessage: "All gateways",
    description: "Findings gateway filter option for every gateway.",
  },
  findingsOutcomeFilter: {
    id: "app.platformPolicies.detail.findingsOutcomeFilter",
    defaultMessage: "Outcome",
    description: "Accessible name of the findings outcome filter.",
  },
  findingsAllOutcomes: {
    id: "app.platformPolicies.detail.findingsAllOutcomes",
    defaultMessage: "All outcomes",
    description: "Findings outcome filter option for every outcome.",
  },
  findingsEmptyTitle: {
    id: "app.platformPolicies.detail.findingsEmptyTitle",
    defaultMessage: "No findings",
    description: "Empty title when no finding matches the filters.",
  },
  findingsEmptyBody: {
    id: "app.platformPolicies.detail.findingsEmptyBody",
    defaultMessage: "No operations match these filters.",
    description: "Empty body when no finding matches the filters.",
  },
  findingsLoading: {
    id: "app.platformPolicies.detail.findingsLoading",
    defaultMessage: "Loading audit findings",
    description: "Accessible label while findings load.",
  },
  findingsError: {
    id: "app.platformPolicies.detail.findingsError",
    defaultMessage: "Could not load audit findings.",
    description: "Text when findings could not be loaded.",
  },
  notFoundTitle: {
    id: "app.platformPolicies.detail.notFoundTitle",
    defaultMessage: "Policy not found",
    description: "Title when the policy does not exist.",
  },
  notFoundBody: {
    id: "app.platformPolicies.detail.notFoundBody",
    defaultMessage: "It may have been deleted.",
    description: "Body when the policy does not exist.",
  },
  backToPolicies: {
    id: "app.platformPolicies.detail.backToPolicies",
    defaultMessage: "Back to platform policies",
    description: "Link back to the policy list.",
  },
});
