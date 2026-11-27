import { defineMessages } from "react-intl";

/*
 * User-facing copy of the docs pages (WEB-UDOC-12, UIS-I18N). Refusal page copy
 * uses plain user language and never names internal components (WEB-UDOC-09).
 */
export const docsMessages = defineMessages({
  pageTitleHome: {
    id: "app.docs.home.title",
    defaultMessage: "Documentation",
    description: "Title of the docs home page and its browser tab",
  },
  pageDescriptionHome: {
    id: "app.docs.home.description",
    defaultMessage: "Step-by-step guides for HyperShell sandboxes and hsctl.",
    description: "One-line subtitle and meta description of the docs home",
  },
  homeGithubCardTitle: {
    id: "app.docs.home.githubCard.title",
    defaultMessage: "GitHub + Jira sandbox",
    description: "Title of the docs home card that opens the GitHub runbook",
  },
  homeGithubCardBody: {
    id: "app.docs.home.githubCard.body",
    defaultMessage: "Set up a sandbox that can reach GitHub and Jira Cloud.",
    description: "Description of the docs home card for the GitHub runbook",
  },
  homeGitlabCardTitle: {
    id: "app.docs.home.gitlabCard.title",
    defaultMessage: "GitLab + Jira sandbox",
    description: "Title of the docs home card that opens the GitLab runbook",
  },
  homeGitlabCardBody: {
    id: "app.docs.home.gitlabCard.body",
    defaultMessage:
      "Set up a sandbox that can reach internal Red Hat GitLab and Jira. Requires Red Hat VPN.",
    description: "Description of the docs home card for the GitLab runbook",
  },
  homeMoreHelp: {
    id: "app.docs.home.moreHelp",
    defaultMessage: "Also see {installLink} and {limitsLink}.",
    description:
      "Muted line under the docs home cards with links to other help pages",
  },
  homeInstallLink: {
    id: "app.docs.home.installLink",
    defaultMessage: "Install hsctl",
    description: "Docs home link to the Install hsctl page",
  },
  homeLimitsLink: {
    id: "app.docs.home.limitsLink",
    defaultMessage: "View resource limit help",
    description: "Docs home link to the resource limit help page",
  },
  backToDocs: {
    id: "app.docs.backToDocs",
    defaultMessage: "All documentation",
    description: "Link from a docs page back to the docs home",
  },
  signInToRun: {
    id: "app.docs.signInToRun",
    defaultMessage: "Sign in to run this step here",
    description:
      "Link shown in place of a runbook Run action when the reader is signed out",
  },
  copyCommand: {
    id: "app.docs.copyCommand",
    defaultMessage: "Copy command: {label}",
    description: "Accessible name of the Copy action of a docs command block",
  },
  stepLabel: {
    id: "app.docs.step.label",
    defaultMessage: "Step {number}",
    description: "Visible label of a runbook step",
  },
  runsInBrowser: {
    id: "app.docs.step.runsInBrowser",
    defaultMessage: "Runs here in your browser",
    description: "States that a runbook step runs in the docs page",
  },
  runsLocally: {
    id: "app.docs.step.runsLocally",
    defaultMessage: "Run in your local terminal",
    description: "States that a runbook step runs in the reader's terminal",
  },
  runbookSteps: {
    id: "app.docs.runbook.steps",
    defaultMessage: "Steps",
    description: "Accessible name of the ordered list of runbook steps",
  },
  run: {
    id: "app.docs.step.run",
    defaultMessage: "Run",
    description: "Primary action that runs a runbook step in the browser",
  },
  running: {
    id: "app.docs.step.running",
    defaultMessage: "Running...",
    description: "Run action label while a runbook step runs",
  },
  recheck: {
    id: "app.docs.step.recheck",
    defaultMessage: "Re-check",
    description:
      "Secondary action that refreshes the gateway after Step 1 reaches Running",
  },
  openInTerminal: {
    id: "app.docs.step.openInTerminal",
    defaultMessage: "Open in terminal",
    description:
      "Link that opens the hsctl terminal at the history entry of a runbook run",
  },
  outputLabel: {
    id: "app.docs.step.output",
    defaultMessage: "Step {number} output",
    description: "Accessible name of the read-only output pane of a step",
  },
  githubRunbookTitle: {
    id: "app.docs.runbook.github.title",
    defaultMessage: "GitHub + Jira sandbox",
    description: "Title of the GitHub and Jira runbook",
  },
  githubRunbookSubtitle: {
    id: "app.docs.runbook.github.subtitle",
    defaultMessage:
      "Create a sandbox that can reach GitHub and Jira Cloud, in three steps.",
    description: "Subtitle of the GitHub and Jira runbook",
  },
  gitlabRunbookTitle: {
    id: "app.docs.runbook.gitlab.title",
    defaultMessage: "GitLab + Jira sandbox",
    description: "Title of the GitLab and Jira runbook",
  },
  gitlabRunbookSubtitle: {
    id: "app.docs.runbook.gitlab.subtitle",
    defaultMessage:
      "Requires Red Hat VPN. Create a sandbox that can reach internal Red Hat GitLab and Jira, in three steps.",
    description:
      "Subtitle of the GitLab and Jira runbook; must state the VPN requirement",
  },
  stepGatewayTitle: {
    id: "app.docs.step.gateway.title",
    defaultMessage: "Choose or create a gateway",
    description: "Title of runbook Step 1 for the public runbook",
  },
  stepVpnGatewayTitle: {
    id: "app.docs.step.vpnGateway.title",
    defaultMessage: "Choose or create a Red Hat VPN gateway",
    description: "Title of runbook Step 1 for the VPN runbook",
  },
  stepGatewayBody: {
    id: "app.docs.step.gateway.body",
    defaultMessage:
      "A gateway runs your sandboxes. Pick one you can already use, or create a new one.",
    description: "Prose of runbook Step 1",
  },
  stepVpnGatewayBody: {
    id: "app.docs.step.vpnGateway.body",
    defaultMessage:
      "Internal Red Hat GitLab is only reachable from a gateway placed on the Red Hat VPN.",
    description: "Prose of runbook Step 1 for the VPN runbook",
  },
  gatewayChoiceLabel: {
    id: "app.docs.step.gateway.choice",
    defaultMessage: "Gateway",
    description: "Label of the gateway choice in runbook Step 1",
  },
  gatewayChoiceNew: {
    id: "app.docs.step.gateway.choiceNew",
    defaultMessage: "Create a new gateway",
    description: "Gateway choice option that creates a new gateway",
  },
  gatewayChoiceLoading: {
    id: "app.docs.step.gateway.choiceLoading",
    defaultMessage: "Loading your gateways...",
    description: "Shown while the existing gateways load in Step 1",
  },
  gatewayChoiceError: {
    id: "app.docs.step.gateway.choiceError",
    defaultMessage:
      "Your gateways could not be loaded. You can still create a new one.",
    description: "Shown when the existing gateways could not be loaded",
  },
  gatewayNameLabel: {
    id: "app.docs.step.gateway.name",
    defaultMessage: "New gateway name",
    description: "Label of the new gateway name field in Step 1",
  },
  gatewayNameHelp: {
    id: "app.docs.step.gateway.nameHelp",
    defaultMessage:
      "Lowercase letters, digits, and hyphens; starts with a letter.",
    description: "Helper text of the new gateway name field",
  },
  gatewayNameInvalid: {
    id: "app.docs.step.gateway.nameInvalid",
    defaultMessage:
      "Use up to 63 lowercase letters, digits, and hyphens, starting with a letter.",
    description: "Validation error of the new gateway name field",
  },
  placementChecking: {
    id: "app.docs.step.placement.checking",
    defaultMessage: "Checking where a new gateway can run...",
    description: "Shown while placement availability loads",
  },
  placementTarget: {
    id: "app.docs.step.placement.target",
    defaultMessage: "A new gateway will run on {placement}.",
    description: "Names the placement a new gateway will use",
  },
  placementKind: {
    id: "app.docs.step.placement.kind",
    defaultMessage: "the local development cluster",
    description: "Placement name for the local Kind cluster",
  },
  placementIbmPublic: {
    id: "app.docs.step.placement.ibmPublic",
    defaultMessage: "IBM Cloud with public internet access",
    description: "Placement name for IBM Cloud public clusters",
  },
  placementAwsPublic: {
    id: "app.docs.step.placement.awsPublic",
    defaultMessage: "AWS with public internet access",
    description: "Placement name for AWS public clusters",
  },
  placementAwsVpn: {
    id: "app.docs.step.placement.awsVpn",
    defaultMessage: "AWS on the Red Hat VPN",
    description: "Placement name for AWS Red Hat VPN clusters",
  },
  publicPlacementUnavailable: {
    id: "app.docs.step.placement.publicUnavailable",
    defaultMessage:
      "No public placement is currently available, so a new gateway cannot be created right now. No cluster with public internet access has room.",
    description:
      "Explains why Run is disabled when no public placement is available",
  },
  vpnPlacementUnavailable: {
    id: "app.docs.step.placement.vpnUnavailable",
    defaultMessage:
      "Red Hat VPN placement is unavailable: no cluster on the Red Hat VPN can take a new gateway right now.",
    description:
      "Explains why Run is disabled when no Red Hat VPN placement is available",
  },
  placementError: {
    id: "app.docs.step.placement.error",
    defaultMessage:
      "Placement availability could not be checked. Try again in a moment.",
    description: "Shown when the placement availability request fails",
  },
  noVpnGateways: {
    id: "app.docs.step.gateway.noVpnGateways",
    defaultMessage:
      "None of your gateways is on the Red Hat VPN, so this step creates a new one.",
    description: "Shown when the reader has no eligible VPN gateway",
  },
  outputCreating: {
    id: "app.docs.output.creating",
    defaultMessage: "Creating gateway {name}...",
    description: "Output line when Step 1 starts creating a gateway",
  },
  outputChecking: {
    id: "app.docs.output.checking",
    defaultMessage: "Checking gateway {name}...",
    description: "Output line when Step 1 checks an existing gateway",
  },
  outputCreated: {
    id: "app.docs.output.created",
    defaultMessage: "Gateway {name} created (ID {id}).",
    description: "Output line when the gateway create request succeeds",
  },
  outputStatus: {
    id: "app.docs.output.status",
    defaultMessage: "Status: {status}",
    description: "Output line for a gateway lifecycle status",
  },
  outputFailed: {
    id: "app.docs.output.failed",
    defaultMessage: "Failed: {reason}",
    description: "Output line when a runbook run fails",
  },
  stageWaitingForPlacement: {
    id: "app.docs.stage.waitingForPlacement",
    defaultMessage: "Waiting for placement",
    description:
      "Lifecycle text for a gateway waiting for a matching cluster (WEB-PLC-07)",
  },
  stagePending: {
    id: "app.docs.stage.pending",
    defaultMessage: "Pending",
    description: "Lifecycle text for a pending gateway",
  },
  stageProvisioning: {
    id: "app.docs.stage.provisioning",
    defaultMessage: "Provisioning",
    description: "Lifecycle text for a provisioning gateway",
  },
  stageRunning: {
    id: "app.docs.stage.running",
    defaultMessage: "Running",
    description: "Lifecycle text for a running gateway",
  },
  stageDegraded: {
    id: "app.docs.stage.degraded",
    defaultMessage: "Degraded",
    description: "Lifecycle text for a degraded gateway",
  },
  stageFailed: {
    id: "app.docs.stage.failed",
    defaultMessage: "Failed",
    description: "Lifecycle text for a failed gateway",
  },
  stageUpdating: {
    id: "app.docs.stage.updating",
    defaultMessage: "Updating",
    description: "Lifecycle text for a gateway in another transitional phase",
  },
  stepStatusAnnouncement: {
    id: "app.docs.step.statusAnnouncement",
    defaultMessage: "Gateway {name}: {status}",
    description: "Polite live announcement of a Step 1 lifecycle change",
  },
  failureConflict: {
    id: "app.docs.failure.conflict",
    defaultMessage:
      "a gateway with this name already exists. Choose another name.",
    description: "Run failure reason for a name conflict",
  },
  failureDenied: {
    id: "app.docs.failure.denied",
    defaultMessage: "you are not allowed to do this.",
    description: "Run failure reason for a denied request",
  },
  failureNotFound: {
    id: "app.docs.failure.notFound",
    defaultMessage: "the gateway no longer exists.",
    description: "Run failure reason for a missing gateway",
  },
  failureUnavailable: {
    id: "app.docs.failure.unavailable",
    defaultMessage: "HyperShell is unavailable. Try again in a moment.",
    description: "Run failure reason for an unavailable service",
  },
  failureUnknown: {
    id: "app.docs.failure.unknown",
    defaultMessage: "something went wrong. Try again.",
    description: "Run failure reason for an unexpected error",
  },
  sessionExpired: {
    id: "app.docs.sessionExpired",
    defaultMessage: "Your console session expired.",
    description: "Shown in a runbook cell when its request needs sign-in",
  },
  stepConnectTitle: {
    id: "app.docs.step.connect.title",
    defaultMessage: "Connect the OpenShell CLI",
    description: "Title of runbook Step 2",
  },
  stepVpnConnectTitle: {
    id: "app.docs.step.vpnConnect.title",
    defaultMessage: "Check the VPN and connect the OpenShell CLI",
    description: "Title of runbook Step 2 for the VPN runbook",
  },
  stepConnectBody: {
    id: "app.docs.step.connect.body",
    defaultMessage:
      "Install the OpenShell CLI that matches your gateway, then register and select the gateway.",
    description: "Prose of runbook Step 2",
  },
  stepConnectPending: {
    id: "app.docs.step.connect.pending",
    defaultMessage:
      "Step 1 supplies these values. The commands appear when the gateway is Running.",
    description:
      "Shown in Step 2 until Step 1 produces a running gateway with connection values",
  },
  vpnCheckBody: {
    id: "app.docs.step.vpnCheck.body",
    defaultMessage:
      "First check that you are on the Red Hat VPN. HTTP 200 or 302 means you are connected. Any other result means you must connect to the Red Hat VPN first.",
    description: "Explains the VPN check command of the VPN runbook",
  },
  vpnCheckLabel: {
    id: "app.docs.step.vpnCheck.label",
    defaultMessage: "Red Hat VPN check",
    description: "Label of the VPN check command block",
  },
  installCliLabel: {
    id: "app.docs.step.installCli.label",
    defaultMessage: "Install the OpenShell CLI",
    description: "Label of the OpenShell CLI install command block",
  },
  selectGatewayLabel: {
    id: "app.docs.step.selectGateway.label",
    defaultMessage: "Register and select the gateway",
    description: "Label of the gateway registration command block",
  },
  stepSandboxTitle: {
    id: "app.docs.step.sandbox.title",
    defaultMessage: "Create the sandbox",
    description: "Title of runbook Step 3",
  },
  stepSandboxBody: {
    id: "app.docs.step.sandbox.body",
    defaultMessage:
      "Set {variables} in your terminal first. The commands read your credentials from your environment; this page never asks for them.",
    description: "Prose of runbook Step 3 naming the credential variables",
  },
  providersLabel: {
    id: "app.docs.step.providers.label",
    defaultMessage: "Create the providers",
    description: "Label of the provider command block",
  },
  sandboxLabel: {
    id: "app.docs.step.sandboxCommand.label",
    defaultMessage: "Create and connect to the sandbox",
    description: "Label of the sandbox command block",
  },
  stepSandboxPlaceholder: {
    id: "app.docs.step.sandbox.placeholder",
    defaultMessage:
      "{placeholder} stands for the gateway name that Step 1 supplies.",
    description: "Names the placeholder that Step 1 replaces",
  },
  installTitle: {
    id: "app.docs.install.title",
    defaultMessage: "Install hsctl",
    description: "Title of the Install hsctl page",
  },
  installSubtitle: {
    id: "app.docs.install.subtitle",
    defaultMessage:
      "Download the hsctl release for your machine, verify it, and add it to your PATH.",
    description: "Subtitle of the Install hsctl page",
  },
  installTabs: {
    id: "app.docs.install.tabs",
    defaultMessage: "Operating system",
    description: "Accessible name of the operating system tabs",
  },
  installMacos: {
    id: "app.docs.install.macos",
    defaultMessage: "macOS",
    description: "macOS tab title",
  },
  installLinux: {
    id: "app.docs.install.linux",
    defaultMessage: "Linux",
    description: "Linux tab title",
  },
  installCommandLabel: {
    id: "app.docs.install.commandLabel",
    defaultMessage: "Install hsctl on {os}",
    description: "Label of the install command block",
  },
  installCommandBody: {
    id: "app.docs.install.commandBody",
    defaultMessage:
      "The command finds your CPU architecture, downloads the matching file and SHA256SUMS from one release, checks the file before it installs anything, installs hsctl in ~/.local/bin, and runs hsctl version.",
    description: "Explains the install command",
  },
  installReleaseConfigured: {
    id: "app.docs.install.releaseConfigured",
    defaultMessage:
      "The command downloads from the hsctl release this console is configured with.",
    description: "Shown when the console has a configured release location",
  },
  installReleaseChoose: {
    id: "app.docs.install.releaseChoose",
    defaultMessage:
      "Open the {releasesLink} and choose the release that matches your HyperShell deployment. Set HSCTL_RELEASE to its tag before you run the command.",
    description:
      "Tells the reader how to choose a release when no location is configured",
  },
  installReleasesLink: {
    id: "app.docs.install.releasesLink",
    defaultMessage: "HyperShell releases page",
    description: "Link text to the repository releases page",
  },
  installWindows: {
    id: "app.docs.install.windows",
    defaultMessage:
      "On Windows, download {asset} from the same release and check it against SHA256SUMS.",
    description: "Note for Windows readers naming the Windows release file",
  },
  openTerminal: {
    id: "app.docs.install.openTerminal",
    defaultMessage: "Open hsctl terminal",
    description:
      "Button on the Install hsctl page that opens the browser hsctl terminal",
  },
  openTerminalBody: {
    id: "app.docs.install.openTerminalBody",
    defaultMessage:
      "Signed in? You can run hsctl commands in your browser without installing anything.",
    description: "Prose next to the Open hsctl terminal button",
  },
  refusalTitle: {
    id: "app.docs.refusal.title",
    defaultMessage: "Your sandbox asked for too much CPU or memory",
    description: "Title of the resource limit refusal page",
  },
  refusalPolicy: {
    id: "app.docs.refusal.policy",
    defaultMessage: "Rule: {policy}",
    description: "Metadata line showing the refusing rule name",
  },
  refusalGateway: {
    id: "app.docs.refusal.gateway",
    defaultMessage: "Gateway: {gateway}",
    description: "Metadata line showing the gateway name",
  },
  refusalContextLabel: {
    id: "app.docs.refusal.context",
    defaultMessage: "Refusal details",
    description: "Accessible name of the refusal metadata line",
  },
  refusalStatement: {
    id: "app.docs.refusal.statement",
    defaultMessage:
      "A platform rule refused your sandbox because of its CPU or memory values. The refusal message can name each value and its limit.",
    description: "Opening statement of the refusal page",
  },
  refusalFixTitle: {
    id: "app.docs.refusal.fixTitle",
    defaultMessage: "Fix it",
    description: "Heading of the refusal page fix section",
  },
  refusalDefaultFixBody: {
    id: "app.docs.refusal.defaultFixBody",
    defaultMessage:
      "Create the sandbox without resource settings. The gateway then applies the platform default resources, which every rule allows.",
    description: "Explains the default resources fix",
  },
  refusalDefaultFixLabel: {
    id: "app.docs.refusal.defaultFixLabel",
    defaultMessage: "Create a sandbox with default resources",
    description: "Label of the default resources command block",
  },
  refusalStandardFixBody: {
    id: "app.docs.refusal.standardFixBody",
    defaultMessage:
      "Need specific values? Start from the standard values below and change them so they stay within the limit in the refusal message.",
    description: "Explains the standard values command",
  },
  refusalStandardFixLabel: {
    id: "app.docs.refusal.standardFixLabel",
    defaultMessage: "Create a sandbox with values to change",
    description: "Label of the standard values command block",
  },
  refusalStandardValues: {
    id: "app.docs.refusal.standardValues",
    defaultMessage:
      "Values to change: CPU request {cpuRequest}, CPU limit {cpuLimit}, memory request {memoryRequest}, memory limit {memoryLimit}.",
    description:
      "Lists the standard resource values marked as values to change",
  },
  refusalCompareTitle: {
    id: "app.docs.refusal.compareTitle",
    defaultMessage: "Blocked and allowed",
    description: "Heading of the blocked and allowed comparison",
  },
  refusalBlocked: {
    id: "app.docs.refusal.blocked",
    defaultMessage: "Blocked",
    description: "Label of the refused request example",
  },
  refusalBlockedBody: {
    id: "app.docs.refusal.blockedBody",
    defaultMessage: "Asks for 8 CPUs and 32 GiB of memory, above the limit.",
    description: "Describes the refused request example",
  },
  refusalAllowed: {
    id: "app.docs.refusal.allowed",
    defaultMessage: "Allowed",
    description: "Label of the allowed request example",
  },
  refusalAllowedBody: {
    id: "app.docs.refusal.allowedBody",
    defaultMessage: "Leaves out resource settings, so the defaults apply.",
    description: "Describes the allowed request example",
  },
  refusalExceptionTitle: {
    id: "app.docs.refusal.exceptionTitle",
    defaultMessage: "Need more than the limit?",
    description: "Heading of the exception request section",
  },
  refusalExceptionBody: {
    id: "app.docs.refusal.exceptionBody",
    defaultMessage:
      "Copy this message, add what you need and why, and send it to the platform team in {supportLink}.",
    description: "Explains how to request an exception",
  },
  refusalSupportLink: {
    id: "app.docs.refusal.supportLink",
    defaultMessage: "the HyperShell support channel",
    description: "Link text to the HyperShell support channel",
  },
  refusalExceptionMessage: {
    id: "app.docs.refusal.exceptionMessage",
    defaultMessage:
      "Hello platform team, a platform rule refused my sandbox because of its CPU or memory values.\nRule: {policy}\nGateway: {gateway}\nI need: [CPU and memory values]\nBecause: [why the defaults are not enough]",
    description:
      "Prefilled exception request message; the square-bracket parts are placeholders the reader replaces",
  },
  refusalExceptionCopy: {
    id: "app.docs.refusal.exceptionCopy",
    defaultMessage: "Copy exception request message",
    description: "Accessible name of the exception message copy action",
  },
  refusalPolicyPlaceholder: {
    id: "app.docs.refusal.policyPlaceholder",
    defaultMessage: "[rule name from the refusal message]",
    description: "Placeholder for a missing rule name in the exception message",
  },
  refusalGatewayPlaceholder: {
    id: "app.docs.refusal.gatewayPlaceholder",
    defaultMessage: "[gateway name]",
    description:
      "Placeholder for a missing gateway name in the exception message",
  },
  refusalDiagramTitle: {
    id: "app.docs.refusal.diagramTitle",
    defaultMessage: "How a sandbox request is checked",
    description: "Title of the refusal diagram",
  },
  refusalDiagramDescription: {
    id: "app.docs.refusal.diagramDescription",
    defaultMessage:
      "Your request goes to the platform rule check. When its CPU and memory values are within the limits, the sandbox is created; when they are not, the sandbox is refused.",
    description: "Text description of the refusal diagram",
  },
  refusalDiagramRequest: {
    id: "app.docs.refusal.diagramRequest",
    defaultMessage: "Your request",
    description: "Refusal diagram node: the reader's request",
  },
  refusalDiagramCheck: {
    id: "app.docs.refusal.diagramCheck",
    defaultMessage: "Platform rule check",
    description: "Refusal diagram node: the rule check",
  },
  refusalDiagramCreated: {
    id: "app.docs.refusal.diagramCreated",
    defaultMessage: "Created (within limits)",
    description: "Refusal diagram outcome when allowed",
  },
  refusalDiagramRefused: {
    id: "app.docs.refusal.diagramRefused",
    defaultMessage: "Refused (over a limit)",
    description: "Refusal diagram outcome when refused",
  },
  copied: {
    id: "app.docs.copied",
    defaultMessage: "Copied",
    description: "Feedback after the exception request message is copied",
  },
  pageError: {
    id: "app.docs.pageError",
    defaultMessage:
      "Part of this page could not load. The commands below still work.",
    description: "Shown on a docs page when a supporting request fails",
  },
});
