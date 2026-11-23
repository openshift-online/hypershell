import { defineMessages } from "react-intl";

/* Copy for the platform policy create/edit editor (WEB-PADM-05..09). */
export const editorMessages = defineMessages({
  pageTitleCreate: {
    id: "app.platformPolicies.editor.pageTitleCreate",
    defaultMessage: "Create platform policy",
    description: "Page title and heading of the create platform policy page.",
  },
  pageTitleEdit: {
    id: "app.platformPolicies.editor.pageTitleEdit",
    defaultMessage: "Edit platform policy",
    description: "Browser title of the edit platform policy page.",
  },
  headingEdit: {
    id: "app.platformPolicies.editor.headingEdit",
    defaultMessage: "Edit {name}",
    description: "Heading of the edit platform policy page.",
  },
  pageDescriptionCreate: {
    id: "app.platformPolicies.editor.pageDescriptionCreate",
    defaultMessage:
      "Write Cedar rules for sandboxes on many gateways. New policies start in audit only.",
    description: "Description under the create platform policy heading.",
  },
  pageDescriptionEdit: {
    id: "app.platformPolicies.editor.pageDescriptionEdit",
    defaultMessage:
      "Change the Cedar rules, message, or gateways of this policy. Its enforcement setting does not change.",
    description: "Description under the edit platform policy heading.",
  },
  pageMetaDescription: {
    id: "app.platformPolicies.editor.pageMetaDescription",
    defaultMessage: "Write and test Cedar rules for a platform policy.",
    description: "Browser meta description of the platform policy editor.",
  },
  rulesSection: {
    id: "app.platformPolicies.editor.rulesSection",
    defaultMessage: "Rules",
    description: "Title of the Cedar rules form section.",
  },
  addRule: {
    id: "app.platformPolicies.editor.addRule",
    defaultMessage: "Add a rule",
    description: "Menu that inserts a Cedar rule from a snippet form.",
  },
  cedarLabel: {
    id: "app.platformPolicies.editor.cedarLabel",
    defaultMessage: "Cedar",
    description: "Label of the Cedar policy editor.",
  },
  cedarEditorAriaLabel: {
    id: "app.platformPolicies.editor.cedarEditorAriaLabel",
    defaultMessage: "Cedar policy source",
    description: "Accessible name of the Cedar code editor.",
  },
  editorLoading: {
    id: "app.platformPolicies.editor.editorLoading",
    defaultMessage: "Loading the Cedar editor",
    description: "Accessible label while the code editor loads.",
  },
  statusValid: {
    id: "app.platformPolicies.editor.statusValid",
    defaultMessage:
      "Valid, {count, plural, one {# statement} other {# statements}}",
    description: "Validation status when the draft is valid.",
  },
  statusProblems: {
    id: "app.platformPolicies.editor.statusProblems",
    defaultMessage: "{count, plural, one {# problem} other {# problems}}",
    description: "Validation status when the draft has problems.",
  },
  statusChecking: {
    id: "app.platformPolicies.editor.statusChecking",
    defaultMessage: "Checking",
    description: "Validation status while the draft is being validated.",
  },
  statusUnavailable: {
    id: "app.platformPolicies.editor.statusUnavailable",
    defaultMessage: "Validation unavailable",
    description: "Validation status when the validation request failed.",
  },
  validHelper: {
    id: "app.platformPolicies.editor.validHelper",
    defaultMessage:
      "Valid against the HyperShell Cedar schema {version}. All statements use forbid.",
    description: "Helper text when the draft passes validation.",
  },
  validHelperNoVersion: {
    id: "app.platformPolicies.editor.validHelperNoVersion",
    defaultMessage:
      "Valid against the HyperShell Cedar schema. All statements use forbid.",
    description:
      "Helper text when the draft passes validation and the schema version is unknown.",
  },
  defaultsPass: {
    id: "app.platformPolicies.editor.defaultsPass",
    defaultMessage:
      "The fleet defaults pass: CPU request {cpu}, memory request {memory}, GPUs 0.",
    description:
      "Helper text confirming the fleet default request is not forbidden.",
  },
  defaultsForbidden: {
    id: "app.platformPolicies.editor.defaultsForbidden",
    defaultMessage:
      "These rules reject the fleet defaults (CPU request {cpu}, memory request {memory}, GPUs 0). Every sandbox that omits a value would be rejected, so you cannot save them.",
    description: "Error when the draft would forbid the fleet default request.",
  },
  fixProblems: {
    id: "app.platformPolicies.editor.fixProblems",
    defaultMessage:
      "Fix {count, plural, one {# problem} other {# problems}} before you save.",
    description: "Helper text when the draft has validation problems.",
  },
  emptyCedar: {
    id: "app.platformPolicies.editor.emptyCedar",
    defaultMessage:
      "Write at least one forbid statement, or use Add a rule to start.",
    description: "Helper text when the Cedar editor is empty.",
  },
  validationUnavailable: {
    id: "app.platformPolicies.editor.validationUnavailable",
    defaultMessage:
      "HyperShell could not validate the rules. Save stays disabled until validation succeeds.",
    description: "Helper text when the validation request failed.",
  },
  problemsTitle: {
    id: "app.platformPolicies.editor.problemsTitle",
    defaultMessage: "{count, plural, one {# problem} other {# problems}}",
    description: "Title of the list of validation problems.",
  },
  diagnosticLine: {
    id: "app.platformPolicies.editor.diagnosticLine",
    defaultMessage: "Line {line}: {message}.",
    description: "One validation problem with its line number.",
  },
  diagnosticWithoutLine: {
    id: "app.platformPolicies.editor.diagnosticWithoutLine",
    defaultMessage: "{message}.",
    description: "One validation problem that applies to no single line.",
  },
  diagnosticSuggestion: {
    id: "app.platformPolicies.editor.diagnosticSuggestion",
    defaultMessage: "Suggested fix: {suggestion}.",
    description: "Suggested fix for a validation problem.",
  },
  previewTitle: {
    id: "app.platformPolicies.editor.previewTitle",
    defaultMessage: "Preview",
    description: "Title of the preview card.",
  },
  previewInvalid: {
    id: "app.platformPolicies.editor.previewInvalid",
    defaultMessage: "Fix the problems in the rules to see the preview.",
    description: "Preview placeholder while the draft is invalid.",
  },
  previewOnTab: {
    id: "app.platformPolicies.editor.previewOnTab",
    defaultMessage: "On the gateway Policies tab",
    description: "Heading of the preview of what gateway users see.",
  },
  previewNoRules: {
    id: "app.platformPolicies.editor.previewNoRules",
    defaultMessage: "No rules yet.",
    description: "Preview when the draft has no statements.",
  },
  previewRange: {
    id: "app.platformPolicies.editor.previewRange",
    defaultMessage: "{minimum} to {maximum}",
    description: "Preview of a resource with a minimum and a maximum.",
  },
  previewUpTo: {
    id: "app.platformPolicies.editor.previewUpTo",
    defaultMessage: "Up to {maximum}",
    description: "Preview of a resource with only a maximum.",
  },
  previewAtLeast: {
    id: "app.platformPolicies.editor.previewAtLeast",
    defaultMessage: "At least {minimum}",
    description: "Preview of a resource with only a minimum.",
  },
  previewAdditionalRule: {
    id: "app.platformPolicies.editor.previewAdditionalRule",
    defaultMessage: "Additional rule",
    description: "Preview term for a rule shown as Cedar.",
  },
  previewLine: {
    id: "app.platformPolicies.editor.previewLine",
    defaultMessage: "Line {line}",
    description: "Line of a rule shown as Cedar in the preview.",
  },
  previewStatement: {
    id: "app.platformPolicies.editor.previewStatement",
    defaultMessage: "Statement {number}",
    description: "Statement number when the line is unknown.",
  },
  previewDenialHeading: {
    id: "app.platformPolicies.editor.previewDenialHeading",
    defaultMessage: "When OpenShell rejects the test request",
    description: "Heading of the denial text preview.",
  },
  previewAllowed: {
    id: "app.platformPolicies.editor.previewAllowed",
    defaultMessage:
      "The test request is allowed. Change it on the Test tab to see the text a user receives when OpenShell rejects a request.",
    description: "Denial preview when the test request is allowed.",
  },
  previewDenialHelper: {
    id: "app.platformPolicies.editor.previewDenialHelper",
    defaultMessage:
      "Statements shown as Cedar can only report the policy name and message, not the failed condition.",
    description: "Helper under the denial text preview.",
  },
  detailsSection: {
    id: "app.platformPolicies.editor.detailsSection",
    defaultMessage: "Details",
    description: "Title of the policy details form section.",
  },
  nameLabel: {
    id: "app.platformPolicies.editor.nameLabel",
    defaultMessage: "Name",
    description: "Label of the policy name field.",
  },
  nameHelperEdit: {
    id: "app.platformPolicies.editor.nameHelperEdit",
    defaultMessage:
      "The name identifies the policy in rejections and cannot change.",
    description: "Helper of the read-only name field when editing.",
  },
  nameRequired: {
    id: "app.platformPolicies.editor.nameRequired",
    defaultMessage: "Enter a name.",
    description: "Error when the policy name is empty.",
  },
  descriptionLabel: {
    id: "app.platformPolicies.editor.descriptionLabel",
    defaultMessage: "Description",
    description: "Label of the policy description field.",
  },
  messageLabel: {
    id: "app.platformPolicies.editor.messageLabel",
    defaultMessage: "Message for users",
    description: "Label of the policy user message field.",
  },
  messageHelper: {
    id: "app.platformPolicies.editor.messageHelper",
    defaultMessage:
      "Shown on the gateway Policies tab and in every rejection from this policy.",
    description: "Helper of the user message field.",
  },
  messageSecretsWarning: {
    id: "app.platformPolicies.editor.messageSecretsWarning",
    defaultMessage:
      "Everyone who uses an affected gateway can read this message. Do not include secrets, credentials, or internal hostnames.",
    description:
      "Warning on the user message field: the message is visible to every affected user.",
  },
  messageRequired: {
    id: "app.platformPolicies.editor.messageRequired",
    defaultMessage: "Enter a message for users.",
    description: "Error when the user message is empty.",
  },
  documentationLabel: {
    id: "app.platformPolicies.editor.documentationLabel",
    defaultMessage: "Documentation URL",
    description: "Label of the policy documentation URL field.",
  },
  appliesToSection: {
    id: "app.platformPolicies.editor.appliesToSection",
    defaultMessage: "Applies to",
    description: "Title of the policy target form section.",
  },
  gatewaysLabel: {
    id: "app.platformPolicies.editor.gatewaysLabel",
    defaultMessage: "Gateways",
    description: "Label of the policy target radio group.",
  },
  targetAll: {
    id: "app.platformPolicies.editor.targetAll",
    defaultMessage: "All gateways",
    description: "Target option: every gateway.",
  },
  targetAllExcept: {
    id: "app.platformPolicies.editor.targetAllExcept",
    defaultMessage: "All gateways except",
    description: "Target option: every gateway except the listed ones.",
  },
  targetSelected: {
    id: "app.platformPolicies.editor.targetSelected",
    defaultMessage: "Selected gateways",
    description: "Target option: only the listed gateways.",
  },
  excludedGateways: {
    id: "app.platformPolicies.editor.excludedGateways",
    defaultMessage: "Excluded gateways",
    description: "Label of the excluded gateway list.",
  },
  selectedGateways: {
    id: "app.platformPolicies.editor.selectedGateways",
    defaultMessage: "Gateways",
    description: "Label of the selected gateway list.",
  },
  findGatewaysExclude: {
    id: "app.platformPolicies.editor.findGatewaysExclude",
    defaultMessage: "Find gateways to exclude",
    description: "Accessible name of the excluded gateway input.",
  },
  findGatewaysSelect: {
    id: "app.platformPolicies.editor.findGatewaysSelect",
    defaultMessage: "Find gateways to include",
    description: "Accessible name of the selected gateway input.",
  },
  findGatewaysPlaceholder: {
    id: "app.platformPolicies.editor.findGatewaysPlaceholder",
    defaultMessage: "Find gateways",
    description: "Placeholder of the gateway input.",
  },
  gatewayInputHelper: {
    id: "app.platformPolicies.editor.gatewayInputHelper",
    defaultMessage: "Type a gateway ID and press Enter to add it.",
    description: "Helper of the gateway input.",
  },
  gatewaysRequired: {
    id: "app.platformPolicies.editor.gatewaysRequired",
    defaultMessage: "Add at least one gateway.",
    description: "Error when Selected gateways has no gateway.",
  },
  removeGateway: {
    id: "app.platformPolicies.editor.removeGateway",
    defaultMessage: "Remove {gateway}",
    description: "Accessible name of the button that removes a gateway chip.",
  },
  createPolicy: {
    id: "app.platformPolicies.editor.createPolicy",
    defaultMessage: "Create policy",
    description: "Button that creates the policy.",
  },
  saveChanges: {
    id: "app.platformPolicies.editor.saveChanges",
    defaultMessage: "Save changes",
    description: "Button that saves an edited policy.",
  },
  cancel: {
    id: "app.platformPolicies.editor.cancel",
    defaultMessage: "Cancel",
    description: "Link that leaves the editor without saving.",
  },
  saveBlockedInvalid: {
    id: "app.platformPolicies.editor.saveBlockedInvalid",
    defaultMessage: "Fix the problems in the rules before you save.",
    description: "Why save is disabled: the draft has validation problems.",
  },
  saveBlockedDefaults: {
    id: "app.platformPolicies.editor.saveBlockedDefaults",
    defaultMessage:
      "Change the rules so they allow the fleet defaults before you save.",
    description: "Why save is disabled: the draft forbids the fleet defaults.",
  },
  saveBlockedPending: {
    id: "app.platformPolicies.editor.saveBlockedPending",
    defaultMessage: "Wait for validation to finish.",
    description: "Why save is disabled: validation is running.",
  },
  saveBlockedFields: {
    id: "app.platformPolicies.editor.saveBlockedFields",
    defaultMessage: "Fill in the required details before you save.",
    description: "Why save is disabled: required fields are empty.",
  },
  saveDeniedTitle: {
    id: "app.platformPolicies.editor.saveDeniedTitle",
    defaultMessage: "You do not have permission to change platform policies",
    description: "Title of the alert when a policy write returns 403.",
  },
  saveDeniedBody: {
    id: "app.platformPolicies.editor.saveDeniedBody",
    defaultMessage:
      "Your platform-policy:admin access may have been removed. Ask a platform administrator for access. Your draft was not saved.",
    description: "Body of the alert when a policy write returns 403.",
  },
  saveConflictTitle: {
    id: "app.platformPolicies.editor.saveConflictTitle",
    defaultMessage: "A policy named {name} already exists",
    description: "Title of the alert when a policy write returns 409.",
  },
  saveConflictBody: {
    id: "app.platformPolicies.editor.saveConflictBody",
    defaultMessage: "Choose another name, or edit the existing policy.",
    description: "Body of the alert when a policy write returns 409.",
  },
  saveInvalidTitle: {
    id: "app.platformPolicies.editor.saveInvalidTitle",
    defaultMessage: "HyperShell rejected the policy",
    description: "Title of the alert when a policy write returns 400.",
  },
  saveFailedTitle: {
    id: "app.platformPolicies.editor.saveFailedTitle",
    defaultMessage: "Could not save the policy",
    description: "Title of the alert when a policy write fails otherwise.",
  },
  saveFailedBody: {
    id: "app.platformPolicies.editor.saveFailedBody",
    defaultMessage: "Try again. Your draft is still here.",
    description: "Body of the alert when a policy write fails otherwise.",
  },
  loadFailedTitle: {
    id: "app.platformPolicies.editor.loadFailedTitle",
    defaultMessage: "Could not load the policy",
    description: "Title when the policy to edit could not be loaded.",
  },
  loadingPolicy: {
    id: "app.platformPolicies.editor.loadingPolicy",
    defaultMessage: "Loading policy",
    description: "Accessible label while the policy to edit loads.",
  },
  retry: {
    id: "app.platformPolicies.editor.retry",
    defaultMessage: "Try again",
    description: "Button that retries loading the policy.",
  },
  sidePanelLabel: {
    id: "app.platformPolicies.editor.sidePanelLabel",
    defaultMessage: "Cedar reference, snippets, and test",
    description: "Accessible name of the editor side panel tabs.",
  },
  tabReference: {
    id: "app.platformPolicies.editor.tabReference",
    defaultMessage: "What you can use",
    description: "Tab with the Cedar language reference.",
  },
  tabSnippets: {
    id: "app.platformPolicies.editor.tabSnippets",
    defaultMessage: "Snippets",
    description: "Tab with the rule snippets.",
  },
  tabTest: {
    id: "app.platformPolicies.editor.tabTest",
    defaultMessage: "Test",
    description: "Tab with the test request panel.",
  },
  referenceStatements: {
    id: "app.platformPolicies.editor.referenceStatements",
    defaultMessage: "Statements",
    description: "Reference heading for statement kinds.",
  },
  referenceStatementsBody: {
    id: "app.platformPolicies.editor.referenceStatementsBody",
    defaultMessage:
      "Use {forbid} only. A request is allowed unless a statement forbids it, so a policy can only add restrictions.",
    description: "Reference text: only forbid statements are allowed.",
  },
  referenceActions: {
    id: "app.platformPolicies.editor.referenceActions",
    defaultMessage: "Actions",
    description: "Reference heading for actions.",
  },
  referenceActionLater: {
    id: "app.platformPolicies.editor.referenceActionLater",
    defaultMessage: "{action} (later)",
    description: "An action planned for a later schema version.",
  },
  referenceContext: {
    id: "app.platformPolicies.editor.referenceContext",
    defaultMessage: "context",
    description: "Reference heading for context attributes (Cedar keyword).",
  },
  referenceAttribute: {
    id: "app.platformPolicies.editor.referenceAttribute",
    defaultMessage: "Attribute",
    description: "Column heading for an attribute name.",
  },
  referenceTypeExample: {
    id: "app.platformPolicies.editor.referenceTypeExample",
    defaultMessage: "Type and example",
    description: "Column heading for an attribute type, unit, and example.",
  },
  referenceContextAttributes: {
    id: "app.platformPolicies.editor.referenceContextAttributes",
    defaultMessage: "Context attributes",
    description: "Accessible name of the context attribute table.",
  },
  referenceTypeUnit: {
    id: "app.platformPolicies.editor.referenceTypeUnit",
    defaultMessage: "{type}, {unit}",
    description: "An attribute type followed by its unit.",
  },
  unitMillicores: {
    id: "app.platformPolicies.editor.unitMillicores",
    defaultMessage: "millicores",
    description: "Unit of CPU attributes.",
  },
  unitBytes: {
    id: "app.platformPolicies.editor.unitBytes",
    defaultMessage: "bytes",
    description: "Unit of memory attributes.",
  },
  unitGpus: {
    id: "app.platformPolicies.editor.unitGpus",
    defaultMessage: "GPUs",
    description: "Unit of the GPU count attribute.",
  },
  unitImageReference: {
    id: "app.platformPolicies.editor.unitImageReference",
    defaultMessage: "image reference",
    description: "Unit of the image attribute.",
  },
  unitSandboxName: {
    id: "app.platformPolicies.editor.unitSandboxName",
    defaultMessage: "sandbox name",
    description: "Unit of the sandbox name attribute.",
  },
  referenceDefaultsHelper: {
    id: "app.platformPolicies.editor.referenceDefaultsHelper",
    defaultMessage:
      "Missing values are filled with the fleet defaults before evaluation, so every attribute is always present.",
    description: "Reference helper about default values.",
  },
  referencePrincipalResource: {
    id: "app.platformPolicies.editor.referencePrincipalResource",
    defaultMessage: "principal and resource",
    description: "Reference heading for principal and resource attributes.",
  },
  typeString: {
    id: "app.platformPolicies.editor.typeString",
    defaultMessage: "String",
    description: "Cedar String type.",
  },
  typeSetOfString: {
    id: "app.platformPolicies.editor.typeSetOfString",
    defaultMessage: "Set of String",
    description: "Cedar Set of String type.",
  },
  typeGatewayId: {
    id: "app.platformPolicies.editor.typeGatewayId",
    defaultMessage: "String (gateway ID)",
    description: "Type of the resource id attribute.",
  },
  typeGatewayName: {
    id: "app.platformPolicies.editor.typeGatewayName",
    defaultMessage: "String (gateway name)",
    description: "Type of the resource name attribute.",
  },
  referenceOperators: {
    id: "app.platformPolicies.editor.referenceOperators",
    defaultMessage: "Operators",
    description: "Reference heading for the allowed operators.",
  },
  referenceNotAvailable: {
    id: "app.platformPolicies.editor.referenceNotAvailable",
    defaultMessage: "Not available",
    description: "Reference heading for constructs not available in v1.",
  },
  unavailablePermit: {
    id: "app.platformPolicies.editor.unavailablePermit",
    defaultMessage: "{permit} statements",
    description: "Unavailable construct: permit statements.",
  },
  unavailableTemplates: {
    id: "app.platformPolicies.editor.unavailableTemplates",
    defaultMessage: "Policy templates and slots",
    description: "Unavailable construct: templates.",
  },
  unavailableQuotas: {
    id: "app.platformPolicies.editor.unavailableQuotas",
    defaultMessage: "Quotas over other sandboxes",
    description: "Unavailable construct: quotas.",
  },
  unavailableTime: {
    id: "app.platformPolicies.editor.unavailableTime",
    defaultMessage: "Time or date conditions",
    description: "Unavailable construct: time conditions.",
  },
  unavailableIp: {
    id: "app.platformPolicies.editor.unavailableIp",
    defaultMessage: "{ip} conditions, until egress rules ship",
    description: "Unavailable construct: the ip() extension.",
  },
  unavailableAnnotations: {
    id: "app.platformPolicies.editor.unavailableAnnotations",
    defaultMessage: "Your own {annotation} annotations (HyperShell adds {id})",
    description: "Unavailable construct: author annotations.",
  },
  snippetsIntro: {
    id: "app.platformPolicies.editor.snippetsIntro",
    defaultMessage:
      "Snippets ask for Kubernetes quantities and insert Cedar that the gateway Policies tab can show in plain words.",
    description: "Introduction of the snippets panel.",
  },
  snippetAdd: {
    id: "app.platformPolicies.editor.snippetAdd",
    defaultMessage: "Add",
    description: "Button that opens a snippet form.",
  },
  snippetAddAriaLabel: {
    id: "app.platformPolicies.editor.snippetAddAriaLabel",
    defaultMessage: "Add {snippet}",
    description: "Accessible name of the button that opens a snippet form.",
  },
  snippetCpuRequestRange: {
    id: "app.platformPolicies.editor.snippetCpuRequestRange",
    defaultMessage: "CPU request range",
    description: "Snippet title.",
  },
  snippetCpuRequestRangeDescription: {
    id: "app.platformPolicies.editor.snippetCpuRequestRangeDescription",
    defaultMessage: "Minimum and maximum CPU request.",
    description: "Snippet description.",
  },
  snippetCpuLimitMax: {
    id: "app.platformPolicies.editor.snippetCpuLimitMax",
    defaultMessage: "CPU limit maximum",
    description: "Snippet title.",
  },
  snippetCpuLimitMaxDescription: {
    id: "app.platformPolicies.editor.snippetCpuLimitMaxDescription",
    defaultMessage: "Largest CPU limit.",
    description: "Snippet description.",
  },
  snippetMemoryRequestRange: {
    id: "app.platformPolicies.editor.snippetMemoryRequestRange",
    defaultMessage: "Memory request range",
    description: "Snippet title.",
  },
  snippetMemoryRequestRangeDescription: {
    id: "app.platformPolicies.editor.snippetMemoryRequestRangeDescription",
    defaultMessage: "Minimum and maximum memory request.",
    description: "Snippet description.",
  },
  snippetMemoryLimitMax: {
    id: "app.platformPolicies.editor.snippetMemoryLimitMax",
    defaultMessage: "Memory limit maximum",
    description: "Snippet title.",
  },
  snippetMemoryLimitMaxDescription: {
    id: "app.platformPolicies.editor.snippetMemoryLimitMaxDescription",
    defaultMessage: "Largest memory limit.",
    description: "Snippet description.",
  },
  snippetGpuMax: {
    id: "app.platformPolicies.editor.snippetGpuMax",
    defaultMessage: "GPU maximum",
    description: "Snippet title.",
  },
  snippetGpuMaxDescription: {
    id: "app.platformPolicies.editor.snippetGpuMaxDescription",
    defaultMessage: "Largest GPU count.",
    description: "Snippet description.",
  },
  snippetImageRegistries: {
    id: "app.platformPolicies.editor.snippetImageRegistries",
    defaultMessage: "Allowed image registries",
    description: "Snippet title.",
  },
  snippetImageRegistriesDescription: {
    id: "app.platformPolicies.editor.snippetImageRegistriesDescription",
    defaultMessage: "Reject images from other registries.",
    description: "Snippet description.",
  },
  snippetGpuRoles: {
    id: "app.platformPolicies.editor.snippetGpuRoles",
    defaultMessage: "Roles that may use GPUs",
    description: "Snippet title.",
  },
  snippetGpuRolesDescription: {
    id: "app.platformPolicies.editor.snippetGpuRolesDescription",
    defaultMessage: "Allow GPUs only for some roles.",
    description: "Snippet description.",
  },
  snippetModalTitle: {
    id: "app.platformPolicies.editor.snippetModalTitle",
    defaultMessage: "Add rule: {snippet}",
    description: "Title of the snippet form dialog.",
  },
  snippetModalDescription: {
    id: "app.platformPolicies.editor.snippetModalDescription",
    defaultMessage:
      "Enter Kubernetes quantities. HyperShell converts them for Cedar.",
    description: "Description of the snippet form dialog.",
  },
  snippetMinimum: {
    id: "app.platformPolicies.editor.snippetMinimum",
    defaultMessage: "Minimum",
    description: "Snippet field: smallest allowed value.",
  },
  snippetMaximum: {
    id: "app.platformPolicies.editor.snippetMaximum",
    defaultMessage: "Maximum",
    description: "Snippet field: largest allowed value.",
  },
  snippetRegistries: {
    id: "app.platformPolicies.editor.snippetRegistries",
    defaultMessage: "Registries",
    description: "Snippet field: allowed image registries.",
  },
  snippetRegistriesHelper: {
    id: "app.platformPolicies.editor.snippetRegistriesHelper",
    defaultMessage:
      "Separate registries with commas, for example quay.io/my-org, registry.redhat.io.",
    description: "Helper of the registries snippet field.",
  },
  snippetRoles: {
    id: "app.platformPolicies.editor.snippetRoles",
    defaultMessage: "Roles",
    description: "Snippet field: roles that may use GPUs.",
  },
  snippetRolesHelper: {
    id: "app.platformPolicies.editor.snippetRolesHelper",
    defaultMessage: "Separate roles with commas, for example gpu-users.",
    description: "Helper of the roles snippet field.",
  },
  snippetRangeHelper: {
    id: "app.platformPolicies.editor.snippetRangeHelper",
    defaultMessage: "Leave a field empty for no bound on that side.",
    description: "Helper of range snippets.",
  },
  snippetOperations: {
    id: "app.platformPolicies.editor.snippetOperations",
    defaultMessage: "Applies to operations",
    description: "Label of the operations a snippet rule applies to.",
  },
  snippetPreview: {
    id: "app.platformPolicies.editor.snippetPreview",
    defaultMessage: "Cedar to insert",
    description: "Label of the Cedar preview in the snippet form.",
  },
  snippetPreviewEmpty: {
    id: "app.platformPolicies.editor.snippetPreviewEmpty",
    defaultMessage: "Enter a value to see the Cedar.",
    description: "Snippet preview before a valid value is entered.",
  },
  snippetInsert: {
    id: "app.platformPolicies.editor.snippetInsert",
    defaultMessage: "Insert rule",
    description: "Button that inserts the snippet rule.",
  },
  errorInvalidCpu: {
    id: "app.platformPolicies.editor.errorInvalidCpu",
    defaultMessage: "Enter a CPU quantity such as 250m or 4.",
    description: "Error for an invalid CPU quantity.",
  },
  errorInvalidMemory: {
    id: "app.platformPolicies.editor.errorInvalidMemory",
    defaultMessage: "Enter a memory quantity such as 512Mi or 16Gi.",
    description: "Error for an invalid memory quantity.",
  },
  errorInvalidCount: {
    id: "app.platformPolicies.editor.errorInvalidCount",
    defaultMessage: "Enter a whole number such as 1.",
    description: "Error for an invalid count.",
  },
  errorMinimumAboveMaximum: {
    id: "app.platformPolicies.editor.errorMinimumAboveMaximum",
    defaultMessage: "The minimum must not be larger than the maximum.",
    description: "Error when a range minimum exceeds its maximum.",
  },
  errorMissingValue: {
    id: "app.platformPolicies.editor.errorMissingValue",
    defaultMessage: "Enter a value.",
    description: "Error when a snippet field is empty.",
  },
  testIntro: {
    id: "app.platformPolicies.editor.testIntro",
    defaultMessage:
      "Evaluate a test request against this draft. Empty fields use the fleet defaults.",
    description: "Introduction of the test panel.",
  },
  testCpuRequest: {
    id: "app.platformPolicies.editor.testCpuRequest",
    defaultMessage: "CPU request",
    description: "Test field label.",
  },
  testCpuLimit: {
    id: "app.platformPolicies.editor.testCpuLimit",
    defaultMessage: "CPU limit",
    description: "Test field label.",
  },
  testMemoryRequest: {
    id: "app.platformPolicies.editor.testMemoryRequest",
    defaultMessage: "Memory request",
    description: "Test field label.",
  },
  testMemoryLimit: {
    id: "app.platformPolicies.editor.testMemoryLimit",
    defaultMessage: "Memory limit",
    description: "Test field label.",
  },
  testGpus: {
    id: "app.platformPolicies.editor.testGpus",
    defaultMessage: "GPUs",
    description: "Test field label.",
  },
  testImage: {
    id: "app.platformPolicies.editor.testImage",
    defaultMessage: "Image",
    description: "Test field label.",
  },
  testRoles: {
    id: "app.platformPolicies.editor.testRoles",
    defaultMessage: "User roles",
    description: "Test field label.",
  },
  testDefaultPlaceholder: {
    id: "app.platformPolicies.editor.testDefaultPlaceholder",
    defaultMessage: "{value} (default)",
    description: "Placeholder showing the fleet default for a test field.",
  },
  testRolesPlaceholder: {
    id: "app.platformPolicies.editor.testRolesPlaceholder",
    defaultMessage: "developers, gpu-users",
    description: "Placeholder of the roles test field.",
  },
  testAllowed: {
    id: "app.platformPolicies.editor.testAllowed",
    defaultMessage: "Allowed",
    description: "Test result when no statement rejects the request.",
  },
  testAllowedBody: {
    id: "app.platformPolicies.editor.testAllowedBody",
    defaultMessage: "No statement in this draft rejects the test request.",
    description: "Body of the allowed test result.",
  },
  testRejected: {
    id: "app.platformPolicies.editor.testRejected",
    defaultMessage:
      "Rejected by {count, plural, one {# statement} other {# statements}}",
    description: "Test result when statements reject the request.",
  },
  testRejectionLine: {
    id: "app.platformPolicies.editor.testRejectionLine",
    defaultMessage: "Line {line}: {rule}",
    description: "A rejecting statement with its line.",
  },
  testRejectionStatement: {
    id: "app.platformPolicies.editor.testRejectionStatement",
    defaultMessage: "Statement {number}: {rule}",
    description: "A rejecting statement when its line is unknown.",
  },
  testInvalidDraft: {
    id: "app.platformPolicies.editor.testInvalidDraft",
    defaultMessage: "Fix the problems in the rules to test a request.",
    description: "Test panel placeholder while the draft is invalid.",
  },
  testChecking: {
    id: "app.platformPolicies.editor.testChecking",
    defaultMessage: "Evaluating the test request",
    description: "Accessible label while the test request is evaluated.",
  },
});
