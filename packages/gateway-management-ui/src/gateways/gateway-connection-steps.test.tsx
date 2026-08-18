import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { IntlProvider } from "react-intl";
import { describe, expect, it } from "vitest";

import { GatewayConnectionSteps } from "./gateway-connection-steps";
import {
  buildSandboxCreateCommand,
  type GatewayConnection,
  installDocsUrl,
} from "./gateway-connections";

const readyGateway: GatewayConnection = {
  clusterName: "Hub cluster",
  consoleUrl: "https://console.example.test",
  endpoint: "https://gateway.example.test:443",
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

  it("renders a prerequisite alert with an install docs link", () => {
    renderSteps(readyGateway);

    expect(screen.getByText("Prerequisite")).toBeTruthy();
    expect(
      screen.getByText(
        /OpenShell CLI must be installed before running the commands below/,
      ),
    ).toBeTruthy();

    const link = screen.getByRole("link", {
      name: /Install the OpenShell CLI/,
    });
    expect(link.getAttribute("href")).toBe(installDocsUrl);
    expect(link.getAttribute("target")).toBe("_blank");
    expect(link.getAttribute("rel")).toContain("noopener");
  });

  it("highlights both command blocks with Shiki once they resolve", async () => {
    const { container } = renderSteps(readyGateway);

    await waitFor(() => {
      expect(container.querySelectorAll(".shiki").length).toBe(2);
    });
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
      expect(container.querySelectorAll(".shiki").length).toBe(2);
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

  it("shows a pending placeholder until the gateway is ready", () => {
    renderSteps({
      ...readyGateway,
      endpoint: undefined,
      phase: "Provisioning",
    });

    expect(screen.getByRole("status")).toBeTruthy();
    expect(
      screen.queryByRole("button", {
        name: "Copy the one-time setup commands",
      }),
    ).toBeNull();
  });
});
