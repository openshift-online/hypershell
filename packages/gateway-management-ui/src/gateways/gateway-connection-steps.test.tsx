import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { IntlProvider } from "react-intl";
import { describe, expect, it } from "vitest";

import { GatewayConnectionSteps } from "./gateway-connection-steps";
import {
  buildOpenShellInstallCommand,
  buildSandboxCreateCommand,
  type GatewayConnection,
  installDocsUrl,
} from "./gateway-connections";

const readyGateway: GatewayConnection = {
  clusterName: "Hub cluster",
  consoleUrl: "https://console.example.test",
  endpoint: "https://gateway.example.test:443",
  gatewayVersion: "0.0.109",
  id: "gateway-1",
  name: "gateway-1",
  oidcAudience: "openshell-cli",
  oidcClientId: "openshell-cli",
  oidcIssuer: "https://issuer.example.test/realms/openshell",
  phase: "Running",
  status: "Running",
};

function renderSteps(gateway: GatewayConnection) {
  return render(
    <IntlProvider locale="en">
      <GatewayConnectionSteps gateway={gateway} />
    </IntlProvider>,
  );
}

describe("GatewayConnectionSteps", () => {
  it("renders the setup and sandbox steps", () => {
    renderSteps(readyGateway);

    expect(
      screen.getByRole("heading", { name: "One-time setup" }),
    ).toBeTruthy();
    expect(
      screen.getByRole("heading", { name: "Create a sandbox" }),
    ).toBeTruthy();
  });

  it("renders installation before the combined setup commands", () => {
    const { container } = renderSteps(readyGateway);

    expect(screen.getByText("Prerequisite")).toBeTruthy();
    expect(
      screen.getByText(
        /Install the OpenShell CLI version for this gateway before you add the provider/,
      ),
    ).toBeTruthy();

    const link = screen.getByRole("link", {
      name: "View installation documentation (opens in a new tab)",
    });
    expect(link.getAttribute("href")).toBe(installDocsUrl);
    expect(link.getAttribute("target")).toBe("_blank");
    expect(link.getAttribute("rel")).toContain("noopener");
    expect(link.classList.contains("pf-m-small")).toBe(true);
    expect(link.textContent).toContain("View installation documentation");

    const installCommand = buildOpenShellInstallCommand(readyGateway);
    expect(installCommand).toBeDefined();

    const commandBlocks = Array.from(container.querySelectorAll("code"));
    const registrationIndex = commandBlocks.findIndex((code) =>
      code.textContent.includes("openshell gateway add"),
    );
    const installationIndex = commandBlocks.findIndex(
      (code) => code.textContent === installCommand,
    );
    const providerIndex = commandBlocks.findIndex((code) =>
      code.textContent.includes("openshell provider create"),
    );

    expect(installationIndex).toBeGreaterThanOrEqual(0);
    expect(installationIndex).toBeLessThan(registrationIndex);
    expect(registrationIndex).toBe(providerIndex);

    const installationCode = commandBlocks[installationIndex];
    expect(installationCode?.textContent).toContain("\\\n");
    expect(installationCode?.textContent.split("\n")).toHaveLength(3);
    expect(
      installationCode
        ?.closest(".pf-v6-c-code-block")
        ?.querySelector("ol, [data-line-number]"),
    ).toBeNull();
  });

  it("copies the version-matched installation command", async () => {
    const user = userEvent.setup();
    renderSteps(readyGateway);

    await user.click(
      screen.getByRole("button", {
        name: "Copy the OpenShell installation command",
      }),
    );

    expect(await navigator.clipboard.readText()).toBe(
      buildOpenShellInstallCommand(readyGateway),
    );
  });

  it("highlights all command blocks with Shiki", async () => {
    const { container } = renderSteps(readyGateway);

    await waitFor(() => {
      expect(container.querySelectorAll(".shiki")).toHaveLength(3);
    });

    const highlightedCommands = Array.from(
      container.querySelectorAll<HTMLElement>(".shiki"),
      (block) => block.textContent,
    );
    expect(
      highlightedCommands.some((command) =>
        command.includes("openshell gateway add"),
      ),
    ).toBe(true);
    expect(
      highlightedCommands.some((command) =>
        command.includes("OPENSHELL_VERSION=v0.0.109"),
      ),
    ).toBe(true);
  });

  it("copies the raw sandbox command, not the highlighted markup", async () => {
    const user = userEvent.setup();
    renderSteps(readyGateway);

    await user.click(
      screen.getByRole("button", { name: "Copy the create-sandbox command" }),
    );

    expect(await navigator.clipboard.readText()).toBe(
      buildSandboxCreateCommand(),
    );
  });

  it("edits the provider name once and mirrors it into both setup slots", async () => {
    const user = userEvent.setup();
    const { container } = renderSteps(readyGateway);

    await waitFor(() => {
      expect(container.querySelectorAll(".shiki")).toHaveLength(3);
    });

    const providerFields = screen.getAllByRole("textbox", {
      name: "Provider name (editable)",
    });
    expect(providerFields).toHaveLength(2);

    const [firstProvider] = providerFields;
    if (firstProvider) {
      firstProvider.textContent = "acme";
      fireEvent.input(firstProvider);
    }

    await waitFor(() => {
      expect(providerFields[1]?.textContent).toBe("acme");
    });

    await user.click(
      screen.getByRole("button", { name: "Copy the one-time setup commands" }),
    );

    const copied = await navigator.clipboard.readText();
    expect(copied).toContain("openshell gateway add");
    expect(copied).toContain("--name acme");
    expect(copied).toContain("--provider acme");
  });

  it("edits the sandbox name and copies the resolved command", async () => {
    const user = userEvent.setup();
    renderSteps(readyGateway);

    const sandboxField = await screen.findByRole("textbox", {
      name: "Sandbox name (editable)",
    });
    sandboxField.textContent = "scratch";
    fireEvent.input(sandboxField);

    await user.click(
      screen.getByRole("button", { name: "Copy the create-sandbox command" }),
    );

    expect(await navigator.clipboard.readText()).toBe(
      buildSandboxCreateCommand("scratch"),
    );
  });

  it("hides the installation prerequisite until the gateway is ready", () => {
    renderSteps({
      ...readyGateway,
      phase: "Provisioning",
      status: "Provisioning",
    });

    expect(screen.getByRole("status")).toBeTruthy();
    expect(screen.queryByText("Prerequisite")).toBeNull();
    expect(
      screen.queryByText(
        /Install the OpenShell CLI version for this gateway before you add the provider/,
      ),
    ).toBeNull();
    expect(
      screen.queryByRole("link", {
        name: "View installation documentation (opens in a new tab)",
      }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", {
        name: "Copy the one-time setup commands",
      }),
    ).toBeNull();
    expect(
      screen.queryByText(/OPENSHELL_VERSION=/, { selector: "code" }),
    ).toBeNull();
  });

  it("hides the installation prerequisite without a reconciled version", () => {
    renderSteps({ ...readyGateway, gatewayVersion: undefined });

    expect(screen.queryByText("Prerequisite")).toBeNull();
    expect(
      screen.queryByRole("link", {
        name: "View installation documentation (opens in a new tab)",
      }),
    ).toBeNull();
    expect(
      screen.getByRole("button", { name: "Copy the one-time setup commands" }),
    ).toBeTruthy();
  });
});
