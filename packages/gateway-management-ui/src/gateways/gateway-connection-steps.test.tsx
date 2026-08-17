import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { IntlProvider } from "react-intl";
import { describe, expect, it } from "vitest";

import { GatewayConnectionSteps } from "./gateway-connection-steps";
import {
  buildSandboxCreateCommand,
  type GatewayConnection,
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
