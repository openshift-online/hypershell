import {
  buildSandboxDriverConfigJson,
  externalLinkUrls,
  sandboxResourceDefaults,
  shellArgument,
} from "@openshift-online/hypershell-gateway-management-ui";

import type { RunbookId } from "./runbook-placement";

/**
 * Placeholder for a gateway name that Step 1 has not supplied yet. It is one
 * plain shell word, so a copied command stays inert, and its name tells the
 * reader which step supplies the value (WEB-UDOC-05).
 */
export const gatewayNamePlaceholder = "GATEWAY_NAME_FROM_STEP_1";

interface RunbookProvider {
  readonly credentialVariable: string;
  readonly name: string;
  readonly type: string;
}

const runbookProviders: Readonly<
  Record<RunbookId, readonly RunbookProvider[]>
> = {
  "github-jira": [
    { credentialVariable: "GITHUB_TOKEN", name: "github", type: "github" },
    { credentialVariable: "JIRA_API_TOKEN", name: "jira", type: "generic" },
  ],
  "gitlab-jira": [
    { credentialVariable: "GITLAB_TOKEN", name: "gitlab", type: "gitlab" },
    { credentialVariable: "JIRA_API_TOKEN", name: "jira", type: "generic" },
  ],
};

/** Default sandbox name of each runbook. */
export const runbookSandboxNames: Readonly<Record<RunbookId, string>> = {
  "github-jira": "github-jira",
  "gitlab-jira": "gitlab-jira",
};

/** Environment variables the reader sets before running Step 3. */
export function runbookCredentialVariables(
  runbook: RunbookId,
): readonly string[] {
  return runbookProviders[runbook].map(
    (provider) => provider.credentialVariable,
  );
}

function gatewayArgument(gatewayName: string | undefined): string {
  return gatewayName === undefined
    ? gatewayNamePlaceholder
    : shellArgument(gatewayName);
}

/**
 * OpenShell provider commands that read each credential from the reader's own
 * environment (WEB-UDOC-06 Step 3). The page never asks for, shows, or stores
 * a credential: each command stops with a message when its variable is unset.
 */
export function buildRunbookProviderCommands(
  runbook: RunbookId,
  gatewayName: string | undefined,
): string {
  const gateway = gatewayArgument(gatewayName);
  return runbookProviders[runbook]
    .map((provider) =>
      [
        `: "\${${provider.credentialVariable}:?Set ${provider.credentialVariable} in this shell first}" \\`,
        "  && openshell provider create \\",
        `  -g ${gateway} \\`,
        `  --name ${provider.name} \\`,
        `  --type ${provider.type} \\`,
        `  --credential ${provider.credentialVariable}`,
      ].join("\n"),
    )
    .join("\n\n");
}

/**
 * Creates the runbook sandbox with the console's standard resource requests
 * and limits (WEB-UIEX-05) and the runbook providers, then connects to it.
 */
export function buildRunbookSandboxCommand(
  runbook: RunbookId,
  gatewayName: string | undefined,
): string {
  const gateway = gatewayArgument(gatewayName);
  const sandbox = runbookSandboxNames[runbook];
  return [
    `DRIVER_CONFIG='${buildSandboxDriverConfigJson()}'`,
    "",
    "openshell sandbox create \\",
    `  -g ${gateway} \\`,
    `  --name ${sandbox} \\`,
    '  --driver-config-json "$DRIVER_CONFIG" \\',
    ...runbookProviders[runbook].map(
      (provider) => `  --provider ${provider.name} \\`,
    ),
    "  --no-auto-providers",
    `openshell sandbox connect -g ${gateway} ${sandbox}`,
  ].join("\n");
}

/** Lines of the commented placeholder shown until Step 1 supplies a gateway. */
export function buildConnectPlaceholder(): string {
  return [
    "# Step 1 supplies the gateway name, endpoint, and sign-in values.",
    "# Run Step 1 to fill in this command:",
    `# openshell gateway add --name ${gatewayNamePlaceholder} GATEWAY_ENDPOINT_FROM_STEP_1`,
  ].join("\n");
}

/**
 * Checks that the reader's machine reaches internal Red Hat GitLab, which is
 * only reachable on the Red Hat VPN (WEB-UDOC-07 Step 2).
 */
export function buildVpnCheckCommand(): string {
  return `curl -sS -o /dev/null -w '%{http_code}\\n' ${externalLinkUrls.redHatInternalGitLab}`;
}

/** Standard resource values shown on the refusal page (WEB-UDOC-09). */
export const standardSandboxResources = sandboxResourceDefaults;

/** Sandbox create with no resource settings: the gateway applies defaults. */
export function buildDefaultResourcesSandboxCommand(): string {
  return "openshell sandbox create --name my-sandbox";
}

/**
 * Sandbox create with the console's standard values, for the reader to change
 * so they stay within the limit in the refusal message.
 */
export function buildStandardResourcesSandboxCommand(): string {
  return [
    `DRIVER_CONFIG='${buildSandboxDriverConfigJson()}'`,
    "",
    "openshell sandbox create \\",
    "  --name my-sandbox \\",
    '  --driver-config-json "$DRIVER_CONFIG"',
  ].join("\n");
}
