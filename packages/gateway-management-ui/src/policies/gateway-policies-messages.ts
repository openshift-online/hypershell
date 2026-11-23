import { defineMessages } from "react-intl";

/** Copy for the read-only gateway Policies tab (WEB-GLIS-26). */
export const gatewayPoliciesMessages = defineMessages({
  tab: {
    id: "app.gateway.policies.tab",
    defaultMessage: "Policies",
    description: "Title of the gateway detail Policies tab.",
  },
  description: {
    id: "app.gateway.policies.description",
    defaultMessage:
      "Platform administrators set these rules for sandboxes on this gateway.",
    description: "Description under the Policies tab heading.",
  },
  refresh: {
    id: "app.gateway.policies.refresh",
    defaultMessage: "Refresh policies",
    description: "Accessible name of the Policies tab refresh button.",
  },
  limitsTitle: {
    id: "app.gateway.policies.limitsTitle",
    defaultMessage: "Sandbox limits on this gateway",
    description: "Title of the card showing effective sandbox limits.",
  },
  limitsEnforced: {
    id: "app.gateway.policies.limitsEnforced",
    defaultMessage:
      "OpenShell rejects sandbox operations outside these limits and adds the fleet default when a value is missing.",
    description: "Limits card description when platform enforcement is on.",
  },
  limitsAudit: {
    id: "app.gateway.policies.limitsAudit",
    defaultMessage:
      "OpenShell rejects sandbox operations outside enforced limits. Values marked Audit only are recorded but not rejected yet. Missing values receive the fleet default.",
    description: "Limits card description when platform enforcement is audit.",
  },
  limitsTableLabel: {
    id: "app.gateway.policies.limitsTableLabel",
    defaultMessage: "Sandbox limits on this gateway",
    description: "Accessible name of the limits matrix.",
  },
  additionalRulesTitle: {
    id: "app.gateway.policies.additionalRulesTitle",
    defaultMessage:
      "{count, plural, one {# additional rule applies} other {# additional rules apply}}",
    description:
      "Title of the alert listing rules the console cannot show as limits.",
  },
  additionalRuleItem: {
    id: "app.gateway.policies.additionalRuleItem",
    defaultMessage: "{message} ({policy})",
    description:
      "One additional rule: the policy message followed by a link to the policy.",
  },
  notApplicable: {
    id: "app.gateway.policies.notApplicable",
    defaultMessage: "-",
    description:
      "Placeholder for a count that does not apply because the policy is enforced.",
  },
  createFitsTitle: {
    id: "app.gateway.policies.createFitsTitle",
    defaultMessage: "Create a sandbox that fits",
    description: "Heading above the compliant sandbox create command.",
  },
  defaultRequestsHelp: {
    id: "app.gateway.policies.defaultRequestsHelp",
    defaultMessage:
      "OpenShell adds the fleet default requests: {cpu} CPU and {memory} memory.",
    description: "Helper text naming the fleet default resource requests.",
  },
  defaultRequestsUnknown: {
    id: "app.gateway.policies.defaultRequestsUnknown",
    defaultMessage: "OpenShell adds the fleet default requests.",
    description:
      "Helper text when the fleet default resource requests are not known.",
  },
  copy: {
    id: "app.gateway.policies.copy",
    defaultMessage: "Copy",
    description: "Hover tip of the copy command button.",
  },
  copied: {
    id: "app.gateway.policies.copied",
    defaultMessage: "Copied",
    description: "Tip shown after the command was copied.",
  },
  policiesTitle: {
    id: "app.gateway.policies.policiesTitle",
    defaultMessage: "Platform policies ({count})",
    description: "Title of the card listing policies on this gateway.",
  },
  policiesTableLabel: {
    id: "app.gateway.policies.policiesTableLabel",
    defaultMessage: "Platform policies on this gateway",
    description: "Accessible name of the policies table.",
  },
  columnPolicy: {
    id: "app.gateway.policies.columnPolicy",
    defaultMessage: "Policy",
    description: "Column heading for the policy name.",
  },
  columnRules: {
    id: "app.gateway.policies.columnRules",
    defaultMessage: "Rules",
    description: "Column heading for the resources a policy bounds.",
  },
  columnEnforcement: {
    id: "app.gateway.policies.columnEnforcement",
    defaultMessage: "Enforcement",
    description: "Column heading for the effective mode.",
  },
  columnWouldReject: {
    id: "app.gateway.policies.columnWouldReject",
    defaultMessage: "Would reject, last 24 hours",
    description: "Column heading for audit-only rejections in the last day.",
  },
  noPoliciesTitle: {
    id: "app.gateway.policies.noPoliciesTitle",
    defaultMessage: "No platform policies",
    description: "Empty state title when no policy applies to the gateway.",
  },
  noPoliciesBody: {
    id: "app.gateway.policies.noPoliciesBody",
    defaultMessage:
      "No platform policy applies to this gateway. Sandboxes receive the fleet defaults when a value is missing.",
    description: "Empty state body when no policy applies to the gateway.",
  },
  loading: {
    id: "app.gateway.policies.loading",
    defaultMessage: "Loading policies",
    description: "Accessible name of the Policies tab loading spinner.",
  },
  loadErrorTitle: {
    id: "app.gateway.policies.loadErrorTitle",
    defaultMessage: "Policies could not be loaded",
    description: "Error title when the effective policies request fails.",
  },
  loadErrorBody: {
    id: "app.gateway.policies.loadErrorBody",
    defaultMessage: "Try again. If the problem continues, contact support.",
    description: "Error body when the effective policies request fails.",
  },
  retry: {
    id: "app.gateway.policies.retry",
    defaultMessage: "Try again",
    description: "Button that retries loading policies.",
  },
  unavailableTitle: {
    id: "app.gateway.policies.unavailableTitle",
    defaultMessage: "Policies are unavailable",
    description:
      "Title when the console host does not provide the policies service.",
  },
  unavailableBody: {
    id: "app.gateway.policies.unavailableBody",
    defaultMessage: "This console cannot read the policies for this gateway.",
    description:
      "Body when the console host does not provide the policies service.",
  },
  findingsTitle: {
    id: "app.gateway.policies.findingsTitle",
    defaultMessage: "Audit findings, last 24 hours",
    description: "Title of the audit findings card.",
  },
  findingsWouldReject: {
    id: "app.gateway.policies.findingsWouldReject",
    defaultMessage:
      "{count, plural, one {# operation would be rejected} other {# operations would be rejected}}",
    description: "Status next to the findings title.",
  },
  findingsIntro: {
    id: "app.gateway.policies.findingsIntro",
    defaultMessage:
      "Audit-only policies allow these operations. They would be rejected or changed when the policy is enforced.",
    description: "Introduction to the audit findings list.",
  },
  allEnforcedTitle: {
    id: "app.gateway.policies.allEnforcedTitle",
    defaultMessage: "No audit-only policies",
    description: "Empty state title when every policy is enforced.",
  },
  allEnforcedBody: {
    id: "app.gateway.policies.allEnforcedBody",
    defaultMessage:
      "Every policy on this gateway is enforced. OpenShell rejects operations that break a rule, so there is nothing to audit.",
    description: "Empty state body when every policy is enforced.",
  },
  whoseOperations: {
    id: "app.gateway.policies.whoseOperations",
    defaultMessage: "Whose operations",
    description: "Accessible name of the scope filter.",
  },
  myOperations: {
    id: "app.gateway.policies.myOperations",
    defaultMessage: "My operations",
    description: "Filter showing only the signed-in user's findings.",
  },
  allOperations: {
    id: "app.gateway.policies.allOperations",
    defaultMessage: "All operations",
    description: "Filter showing every user's findings.",
  },
  outcomeFilter: {
    id: "app.gateway.policies.outcomeFilter",
    defaultMessage: "Outcome",
    description: "Accessible name of the outcome filter.",
  },
  allOutcomes: {
    id: "app.gateway.policies.allOutcomes",
    defaultMessage: "All outcomes",
    description: "Filter showing findings of every outcome.",
  },
  noFindingsTitle: {
    id: "app.gateway.policies.noFindingsTitle",
    defaultMessage: "No findings",
    description: "Empty state title when no finding matches the filters.",
  },
  noFindingsBody: {
    id: "app.gateway.policies.noFindingsBody",
    defaultMessage: "No operations in the last 24 hours match these filters.",
    description: "Empty state body when no finding matches the filters.",
  },
  findingsTableLabel: {
    id: "app.gateway.policies.findingsTableLabel",
    defaultMessage: "Audit findings",
    description: "Accessible name of the findings table.",
  },
  columnTime: {
    id: "app.gateway.policies.columnTime",
    defaultMessage: "Time",
    description: "Column heading for when a finding occurred.",
  },
  columnUser: {
    id: "app.gateway.policies.columnUser",
    defaultMessage: "User",
    description: "Column heading for who made the request.",
  },
  columnOperation: {
    id: "app.gateway.policies.columnOperation",
    defaultMessage: "Operation",
    description: "Column heading for the sandbox operation.",
  },
  columnOutcome: {
    id: "app.gateway.policies.columnOutcome",
    defaultMessage: "Outcome",
    description: "Column heading for the finding outcome.",
  },
  columnDetail: {
    id: "app.gateway.policies.columnDetail",
    defaultMessage: "Detail",
    description: "Column heading for the finding detail.",
  },
  drawerKind: {
    id: "app.gateway.policies.drawerKind",
    defaultMessage: "Platform policy",
    description: "Small heading above the policy name in the drawer.",
  },
  drawerRules: {
    id: "app.gateway.policies.drawerRules",
    defaultMessage: "Rules",
    description: "Heading for the rules section in the drawer.",
  },
  drawerWhy: {
    id: "app.gateway.policies.drawerWhy",
    defaultMessage:
      "{mode, select, enforce {Why it is enforced} other {Why it is audit only}}",
    description: "Heading for the mode explanation in the drawer.",
  },
  lastUpdated: {
    id: "app.gateway.policies.lastUpdated",
    defaultMessage: "Last updated",
    description: "Term for when the policy last changed.",
  },
  closeDrawer: {
    id: "app.gateway.policies.closeDrawer",
    defaultMessage: "Close policy details",
    description: "Accessible name of the drawer close button.",
  },
});
