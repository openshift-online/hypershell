import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { IntlProvider } from "react-intl";
import { MemoryRouter } from "react-router";
import { vi } from "vitest";

import { englishMessages } from "../../i18n/catalog";
import {
  buildGatewayAddCommand,
  previewGateway,
  previewGateways,
  type GatewayConnection,
} from "./gateway-connections";
import { GatewayDetails, GatewayDirectory } from "./gateway-directory";

const expectedCommand =
  "openshell gateway add --name openshell-gateway-test --oidc-issuer https://keycloak-openshell-keycloak.apps.rosa.hcmais01ue1.s9m2.p3.openshiftapps.com/realms/openshell --oidc-client-id openshell-cli --oidc-audience openshell-cli https://openshell-gw-openshell-gateway-test.apps.rosa.hcmais01ue1.s9m2.p3.openshiftapps.com:443";

function renderContent(content: React.ReactNode) {
  return render(
    <IntlProvider locale="en" messages={englishMessages}>
      <MemoryRouter>{content}</MemoryRouter>
    </IntlProvider>,
  );
}

describe("buildGatewayAddCommand", () => {
  it("builds the documented OpenShell connection command", () => {
    expect(buildGatewayAddCommand(previewGateway)).toBe(expectedCommand);
  });

  it("quotes values that could be interpreted by a shell", () => {
    const gateway: GatewayConnection = {
      ...previewGateway,
      name: "gateway $(unsafe)",
    };

    expect(buildGatewayAddCommand(gateway)).toContain(
      "--name 'gateway $(unsafe)'",
    );
  });
});

describe("GatewayDirectory", () => {
  it("gives users both console and CLI connection methods", async () => {
    const user = userEvent.setup();
    const writeText = vi.fn();
    Object.defineProperty(window.navigator, "clipboard", {
      configurable: true,
      value: { writeText },
    });

    renderContent(<GatewayDirectory gateways={previewGateways} />);

    expect(
      screen.getByRole("heading", { level: 1, name: "OpenShell gateways" }),
    ).toBeTruthy();
    expect(
      screen
        .getByRole("link", {
          name: "Open console for openshell-gateway-test in a new tab",
        })
        .getAttribute("href"),
    ).toBe(previewGateway.consoleUrl);
    expect(
      screen
        .getByRole("link", { name: "openshell-gateway-test" })
        .getAttribute("href"),
    ).toBe("/gateways/openshell-gateway-test");

    await user.click(
      screen.getByRole("button", {
        name: "Copy connection command for openshell-gateway-test",
      }),
    );
    expect(writeText).toHaveBeenCalledWith(expectedCommand);
  });

  it("provides guidance when no gateways are visible", () => {
    renderContent(<GatewayDirectory gateways={[]} />);

    expect(
      screen.getByRole("heading", { level: 2, name: "No gateways available" }),
    ).toBeTruthy();
    expect(
      screen.getByText(
        "Ask your OpenShell administrator for access to a gateway.",
      ),
    ).toBeTruthy();
  });
});

describe("GatewayDetails", () => {
  it("presents the same connection command on the detail page", () => {
    renderContent(<GatewayDetails gateway={previewGateway} />);

    expect(
      screen.getByRole("heading", {
        level: 1,
        name: "openshell-gateway-test",
      }),
    ).toBeTruthy();
    expect(screen.getByDisplayValue(expectedCommand)).toBeTruthy();
    expect(screen.getByText("Ready")).toBeTruthy();
  });
});
