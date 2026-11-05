import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { IntlProvider } from "react-intl";
import { describe, expect, it, vi } from "vitest";

import { GatewayConnectionSteps } from "./gateway-connection-steps";
import {
  buildOpenShellInstallCommand,
  type GatewayConnection,
  shellArgument,
} from "./gateway-connections";

const readyGateway: GatewayConnection = {
  clusterId: "cluster-east",
  clusterName: "local-kind",
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

const stepTitles = [
  "Install the OpenShell CLI",
  "Add this gateway",
  "Create the EnMaaS provider profile",
  "Import the EnMaaS provider profile",
  "Connect this gateway to EnMaaS",
  "Create a Claude sandbox",
  "Open Claude in the sandbox",
] as const;

function renderSteps(
  gateway: GatewayConnection = readyGateway,
  isProvisioning = false,
) {
  return render(
    <IntlProvider locale="en">
      <GatewayConnectionSteps
        gateway={gateway}
        isProvisioning={isProvisioning}
      />
    </IntlProvider>,
  );
}

function expandedToggles() {
  return screen
    .getAllByRole("button")
    .filter((button) => button.getAttribute("aria-expanded") === "true");
}

function getStepToggle(title: string) {
  const escapedTitle = title.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  return screen.getByRole("button", {
    name: new RegExp(`^\\d+${escapedTitle}(?:One-time)?$`, "u"),
  });
}

describe("GatewayConnectionSteps", () => {
  it("renders the mockup's seven guided setup steps", () => {
    renderSteps();

    stepTitles.forEach((title) => {
      expect(getStepToggle(title)).toBeTruthy();
    });
    expect(screen.getAllByText("One-time")).toHaveLength(5);
    expect(
      screen.getByRole("button", { name: "Copy all commands" }),
    ).toBeTruthy();
    expect(screen.getByText("Before you begin")).toBeTruthy();
  });

  it("expands only one connection step at a time", async () => {
    const user = userEvent.setup();
    renderSteps();

    expect(expandedToggles()).toHaveLength(0);
    await user.click(getStepToggle("Add this gateway"));
    expect(expandedToggles()).toHaveLength(1);
    expect(
      getStepToggle("Add this gateway").getAttribute("aria-expanded"),
    ).toBe("true");
    expect(
      getStepToggle("Install the OpenShell CLI").getAttribute("aria-expanded"),
    ).toBe("false");
    await user.click(getStepToggle("Add this gateway"));
    expect(expandedToggles()).toHaveLength(0);
  });

  it("copies every resolved command from the guided setup", async () => {
    const user = userEvent.setup();
    renderSteps();

    await user.click(screen.getByRole("button", { name: "Copy all commands" }));

    const copied = await navigator.clipboard.readText();
    expect(copied).toContain(buildOpenShellInstallCommand(readyGateway));
    expect(copied).toContain("openshell gateway add");
    expect(copied).toContain("openshell gateway select gateway-1");
    expect(copied).toContain("cat > enmaas.yaml <<'YAML'");
    expect(copied).toContain(
      "openshell profile import -f enmaas.yaml --global",
    );
    expect(copied).toContain("openshell provider create");
    expect(copied).toContain("--type enmaas");
    expect(copied).toContain("openshell sandbox create");
    expect(copied).toContain("openshell sandbox connect enmaas-claude");
  });

  it("keeps multiline provider and sandbox commands executable", async () => {
    const user = userEvent.setup();
    renderSteps();

    await user.click(screen.getByRole("button", { name: "Copy all commands" }));

    const copied = await navigator.clipboard.readText();
    expect(copied).toContain("openshell provider create \\\n");
    expect(copied).not.toContain("openshell provider create\n");
    expect(copied).toContain("openshell sandbox create \\\n");
    expect(copied).not.toContain("openshell sandbox create\n");
  });

  it("safely encodes a gateway name in the gateway lookup command", async () => {
    const user = userEvent.setup();
    const unsafeGatewayName = 'gateway"; touch compromised; #';
    renderSteps({ ...readyGateway, name: unsafeGatewayName });

    await user.click(screen.getByRole("button", { name: "Copy all commands" }));

    const copied = await navigator.clipboard.readText();
    expect(copied).toContain(
      `grep -qF -- ${shellArgument(`"name": "${unsafeGatewayName}"`)}`,
    );
    expect(copied).toContain(
      `openshell gateway select ${shellArgument(unsafeGatewayName)}`,
    );
  });

  it("reports a failed copy-all operation", async () => {
    const user = userEvent.setup();
    const clipboardWrite = vi
      .spyOn(navigator.clipboard, "writeText")
      .mockRejectedValue(new Error("denied"));
    renderSteps();

    await user.click(screen.getByRole("button", { name: "Copy all commands" }));

    expect(await screen.findByText("Copy failed")).toBeTruthy();
    clipboardWrite.mockRestore();
  });

  it("uses form values in the provider and sandbox commands", async () => {
    const user = userEvent.setup();
    renderSteps();

    await user.click(getStepToggle("Connect this gateway to EnMaaS"));
    await user.clear(screen.getByLabelText("Provider name"));
    await user.type(screen.getByLabelText("Provider name"), "acme");
    await user.click(
      screen.getByRole("button", {
        name: "Copy command for step 5, Connect this gateway to EnMaaS",
      }),
    );
    const providerCommand = await navigator.clipboard.readText();
    expect(providerCommand).toContain("--name acme");
    expect(providerCommand).toContain("openshell provider create");

    await user.click(getStepToggle("Create a Claude sandbox"));
    await user.clear(screen.getByLabelText("Sandbox name"));
    await user.type(screen.getByLabelText("Sandbox name"), "scratch");
    await user.clear(screen.getByLabelText("Claude model"));
    await user.type(screen.getByLabelText("Claude model"), "acme-model");
    await user.click(
      screen.getByRole("button", {
        name: "Copy command for step 6, Create a Claude sandbox",
      }),
    );
    const sandboxCommand = await navigator.clipboard.readText();
    expect(sandboxCommand).toContain("--name scratch");
    expect(sandboxCommand).toContain("--provider acme");
    expect(sandboxCommand).toContain("--model acme-model");

    await user.click(getStepToggle("Open Claude in the sandbox"));
    await user.click(
      screen.getByRole("button", {
        name: "Copy command for step 7, Open Claude in the sandbox",
      }),
    );
    expect(await navigator.clipboard.readText()).toBe(
      "openshell sandbox connect scratch",
    );
  });

  it("keeps the gateway step pending while provisioning", async () => {
    const user = userEvent.setup();
    renderSteps(readyGateway, true);

    await user.click(getStepToggle("Add this gateway"));

    expect(
      screen.getByText("Waiting for gateway provisioning..."),
    ).toBeTruthy();
    expect(
      screen.queryByRole("button", {
        name: "Copy command for step 2, Add this gateway",
      }),
    ).toBeNull();
  });

  it("withholds gateway-dependent commands until provisioning completes", async () => {
    const user = userEvent.setup();
    renderSteps({ ...readyGateway, phase: "Provisioning" }, true);

    const gatewayDependentSteps = [
      { number: 4, title: "Import the EnMaaS provider profile" },
      { number: 5, title: "Connect this gateway to EnMaaS" },
      { number: 6, title: "Create a Claude sandbox" },
      { number: 7, title: "Open Claude in the sandbox" },
    ];

    for (const { number, title } of gatewayDependentSteps) {
      await user.click(getStepToggle(title));
      expect(
        screen.getByText("Waiting for gateway provisioning..."),
      ).toBeTruthy();
      expect(
        screen.queryByRole("button", {
          name: `Copy command for step ${String(number)}, ${title}`,
        }),
      ).toBeNull();
    }

    expect(screen.queryByLabelText("Provider name")).toBeNull();
    expect(screen.queryByLabelText("Sandbox name")).toBeNull();
  });

  it("omits unavailable commands from copy-all", async () => {
    const user = userEvent.setup();
    renderSteps({ ...readyGateway, gatewayVersion: undefined });

    await user.click(screen.getByRole("button", { name: "Copy all commands" }));

    const copied = await navigator.clipboard.readText();
    expect(copied).not.toContain("OPENSHELL_VERSION=");
    expect(copied).toContain("openshell gateway add");
    await waitFor(() => {
      expect(getStepToggle("Install the OpenShell CLI")).toBeTruthy();
    });
  });
});
