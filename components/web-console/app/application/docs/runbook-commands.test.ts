import { describe, expect, it } from "vitest";

import {
  buildConnectPlaceholder,
  buildDefaultResourcesSandboxCommand,
  buildRunbookProviderCommands,
  buildRunbookSandboxCommand,
  buildStandardResourcesSandboxCommand,
  buildVpnCheckCommand,
  gatewayNamePlaceholder,
  runbookCredentialVariables,
} from "./runbook-commands";

describe("runbook commands", () => {
  // Verifies: WEB-UDOC-06
  it("reads GitHub and Jira credentials from the reader's environment", () => {
    const commands = buildRunbookProviderCommands("github-jira", "team-gw");
    expect(commands).toContain('"${GITHUB_TOKEN:?');
    expect(commands).toContain("--type github");
    expect(commands).toContain("--credential GITHUB_TOKEN");
    expect(commands).toContain("--credential JIRA_API_TOKEN");
    expect(commands).toContain("-g team-gw");
    expect(runbookCredentialVariables("github-jira")).toEqual([
      "GITHUB_TOKEN",
      "JIRA_API_TOKEN",
    ]);
  });

  // Verifies: WEB-UDOC-07
  it("uses GitLab in place of GitHub for the VPN runbook", () => {
    const commands = buildRunbookProviderCommands("gitlab-jira", "vpn-gw");
    expect(commands).toContain("--type gitlab");
    expect(commands).not.toContain("--type github");
    expect(buildRunbookSandboxCommand("gitlab-jira", "vpn-gw")).toContain(
      "--provider gitlab",
    );
  });

  // Verifies: WEB-UDOC-06
  it("creates the sandbox with the standard resources and connects to it", () => {
    const command = buildRunbookSandboxCommand("github-jira", "team-gw");
    expect(command).toContain(
      `DRIVER_CONFIG='{"kubernetes":{"containers":{"agent":{"resources":{"requests":{"cpu":"100m","memory":"512Mi"},"limits":{"cpu":"500m","memory":"512Mi"}}}}}}'`,
    );
    expect(command).toContain('--driver-config-json "$DRIVER_CONFIG"');
    expect(command).toContain("--provider github");
    expect(command).toContain("--provider jira");
    expect(command).toContain(
      "openshell sandbox connect -g team-gw github-jira",
    );
  });

  // Verifies: WEB-UDOC-05
  it("marks the placeholder that Step 1 replaces", () => {
    expect(buildRunbookSandboxCommand("github-jira", undefined)).toContain(
      `-g ${gatewayNamePlaceholder}`,
    );
    expect(buildRunbookProviderCommands("github-jira", undefined)).toContain(
      `-g ${gatewayNamePlaceholder}`,
    );
    for (const line of buildConnectPlaceholder().split("\n")) {
      expect(line.startsWith("#")).toBe(true);
    }
  });

  // Verifies: WEB-UDOC-10.01
  it.each(["$(touch /tmp/x)", "a`id`b", "x; rm -rf ~", "it's"])(
    "keeps the gateway name %j one safely quoted shell argument",
    (name) => {
      const command = buildRunbookSandboxCommand("github-jira", name);
      const quoted = `'${name.replaceAll("'", `'"'"'`)}'`;
      expect(command).toContain(`-g ${quoted} \\`);
      expect(buildRunbookProviderCommands("github-jira", name)).toContain(
        `-g ${quoted} \\`,
      );
    },
  );

  // Verifies: WEB-UDOC-07
  it("checks the VPN against the internal GitLab URL from the registry", () => {
    expect(buildVpnCheckCommand()).toBe(
      "curl -sS -o /dev/null -w '%{http_code}\\n' https://gitlab.cee.redhat.com",
    );
  });

  // Verifies: WEB-UDOC-09
  it("offers default and standard resource sandbox commands", () => {
    expect(buildDefaultResourcesSandboxCommand()).toBe(
      "openshell sandbox create --name my-sandbox",
    );
    expect(buildStandardResourcesSandboxCommand()).toContain(
      '"limits":{"cpu":"500m","memory":"512Mi"}',
    );
  });
});
