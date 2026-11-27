import { defineMessages } from "react-intl";

/*
 * Shared policy vocabulary (design council terminology for #1145): "Enforced"
 * and "Audit only" are the two effective modes and are always shown as status
 * labels; "reject" matches the OpenShell denial text; "add the default"
 * describes default injection; "Platform enforcement" is the global minimum.
 */
export const policyMessages = defineMessages({
  modeEnforced: {
    id: "app.policy.mode.enforced",
    defaultMessage: "Enforced",
    description: "Status label for a policy that rejects operations.",
  },
  modeAuditOnly: {
    id: "app.policy.mode.auditOnly",
    defaultMessage: "Audit only",
    description:
      "Status label for a policy that allows operations and records findings.",
  },
  reasonPlatformEnforced: {
    id: "app.policy.reason.platformEnforced",
    defaultMessage: "By platform enforcement",
    description:
      "Reason shown under the mode label when platform enforcement enforces the policy.",
  },
  reasonPolicySetting: {
    id: "app.policy.reason.policySetting",
    defaultMessage: "Policy setting",
    description:
      "Reason shown under the mode label when the policy's own setting decides the mode.",
  },
  reasonCannotEnforce: {
    id: "app.policy.reason.cannotEnforce",
    defaultMessage: "Cannot enforce",
    description:
      "Reason shown when the gateway cannot enforce the policy (unsupported).",
  },
  resourceCpuRequest: {
    id: "app.policy.resource.cpuRequest",
    defaultMessage: "CPU request",
    description: "Name of the sandbox CPU request resource.",
  },
  resourceCpuLimit: {
    id: "app.policy.resource.cpuLimit",
    defaultMessage: "CPU limit",
    description: "Name of the sandbox CPU limit resource.",
  },
  resourceMemoryRequest: {
    id: "app.policy.resource.memoryRequest",
    defaultMessage: "Memory request",
    description: "Name of the sandbox memory request resource.",
  },
  resourceMemoryLimit: {
    id: "app.policy.resource.memoryLimit",
    defaultMessage: "Memory limit",
    description: "Name of the sandbox memory limit resource.",
  },
  resourceGpuCount: {
    id: "app.policy.resource.gpuCount",
    defaultMessage: "GPUs",
    description: "Name of the sandbox GPU count resource.",
  },
  columnResource: {
    id: "app.policy.column.resource",
    defaultMessage: "Resource",
    description: "Column heading for the resource in a rules matrix.",
  },
  columnMinimum: {
    id: "app.policy.column.minimum",
    defaultMessage: "Minimum",
    description: "Column heading for the smallest allowed value.",
  },
  columnDefault: {
    id: "app.policy.column.default",
    defaultMessage: "Default if missing",
    description:
      "Column heading for the value OpenShell adds when a request omits it.",
  },
  columnMaximum: {
    id: "app.policy.column.maximum",
    defaultMessage: "Maximum",
    description: "Column heading for the largest allowed value.",
  },
  noRule: {
    id: "app.policy.noRule",
    defaultMessage: "No rule",
    description: "Placeholder when no rule bounds a resource.",
  },
  rulesInPolicy: {
    id: "app.policy.rulesInPolicy",
    defaultMessage: "Rules in {name}",
    description: "Accessible name for the rules table of one policy.",
  },
  shownAsCedar: {
    id: "app.policy.shownAsCedar",
    defaultMessage: "Shown as Cedar",
    description:
      "Label for a rule the console cannot summarize in plain words, shown as its Cedar source.",
  },
  additionalRule: {
    id: "app.policy.additionalRule",
    defaultMessage: "Additional rule",
    description:
      "Chip for a rule the console cannot summarize as a resource bound.",
  },
  outcomeWouldReject: {
    id: "app.policy.outcome.wouldReject",
    defaultMessage: "Would reject",
    description:
      "Label for an audit finding the policy would have rejected if enforced.",
  },
  outcomeWouldAddDefaults: {
    id: "app.policy.outcome.wouldAddDefaults",
    defaultMessage: "Would add defaults",
    description:
      "Label for an audit finding that would have received default values if enforced.",
  },
  howEnforcementWorks: {
    id: "app.policy.howEnforcementWorks",
    defaultMessage: "How enforcement works",
    description: "Title and accessible name of the enforcement explanation.",
  },
  howEnforcementWorksBody: {
    id: "app.policy.howEnforcementWorksBody",
    defaultMessage:
      "Enforced rules reject sandbox operations that break them and add missing defaults. Audit-only rules allow the operation and record what they would reject.",
    description: "First paragraph of the enforcement explanation.",
  },
  howEnforcementWorksWho: {
    id: "app.policy.howEnforcementWorksWho",
    defaultMessage:
      "A policy is enforced when platform enforcement or the policy itself is set to Enforced. Platform policy administrators manage both.",
    description: "Second paragraph of the enforcement explanation.",
  },
  platformEnforcementIs: {
    id: "app.policy.platformEnforcementIs",
    defaultMessage:
      "Platform enforcement is {mode, select, enforce {Enforced} other {Audit only}}.",
    description: "Current platform enforcement in the enforcement explanation.",
  },
  platformEnforcement: {
    id: "app.policy.platformEnforcement",
    defaultMessage: "Platform enforcement",
    description: "Name of the global minimum enforcement mode.",
  },
  policySetting: {
    id: "app.policy.policySetting",
    defaultMessage: "Policy setting",
    description: "Name of the mode set on the policy itself.",
  },
  modeResult: {
    id: "app.policy.modeResult",
    defaultMessage: "Result",
    description: "The resulting effective mode in the mode explanation.",
  },
  modeWord: {
    id: "app.policy.modeWord",
    defaultMessage: "{mode, select, enforce {Enforced} other {Audit only}}",
    description: "A mode written as a word.",
  },
  messageForUsers: {
    id: "app.policy.messageForUsers",
    defaultMessage: "Message for users",
    description: "Heading for the policy userMessage.",
  },
  policyDocumentation: {
    id: "app.policy.documentation",
    defaultMessage: "Policy documentation",
    description: "Link to the documentation URL of a policy.",
  },
  showCedarSource: {
    id: "app.policy.showCedarSource",
    defaultMessage: "Show Cedar source",
    description: "Toggle that reveals the Cedar source of a policy.",
  },
  hideCedarSource: {
    id: "app.policy.hideCedarSource",
    defaultMessage: "Hide Cedar source",
    description: "Toggle that hides the Cedar source of a policy.",
  },
  findingDetail: {
    id: "app.policy.findingDetail",
    defaultMessage: "{request} ({policies})",
    description:
      "Detail of an audit finding: the request values followed by the policy names.",
  },
  findingRequestValue: {
    id: "app.policy.findingRequestValue",
    defaultMessage: "{resource} {value}",
    description:
      "One request value in an audit finding, for example CPU request 100m.",
  },
  findingNoValues: {
    id: "app.policy.findingNoValues",
    defaultMessage: "No resource values recorded",
    description: "Audit finding detail when the request carried no values.",
  },
  operationCreateSandbox: {
    id: "app.policy.operation.createSandbox",
    defaultMessage: "Create sandbox",
    description: "The CreateSandbox operation named in an audit finding.",
  },
});
