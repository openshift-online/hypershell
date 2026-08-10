import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { vi } from "vitest";

import {
  GatewayUiProvider,
  useGatewayLink,
  useGatewayUi,
} from "./gateway-ui-provider";

const gatewayOperations = {
  findGatewayPlacements: vi.fn(),
  getGateway: vi.fn(),
  getGatewayPlacement: vi.fn(),
  listGateways: vi.fn(),
  provisionGateway: vi.fn(),
  removeGateway: vi.fn(),
  renameGateway: vi.fn(),
};

const navigate = vi.fn();
const gatewayLabel = "Gateway";
const navigation = {
  collectionHref: "/",
  createHref: "/gateways/new",
  detailHref: (gatewayId: string) => `/gateways/${gatewayId}`,
  navigate,
};

function TestLink() {
  const link = useGatewayLink("#gateway-1");

  return <a {...link}>{gatewayLabel}</a>;
}

function ServicesConsumer() {
  const services = useGatewayUi();

  return <span>{services.navigation.createHref}</span>;
}

describe("GatewayUiProvider", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("exposes host services and intercepts an unmodified link activation", async () => {
    const user = userEvent.setup();
    render(
      <GatewayUiProvider gateways={gatewayOperations} navigation={navigation}>
        <ServicesConsumer />
        <TestLink />
      </GatewayUiProvider>,
    );

    expect(screen.getByText("/gateways/new")).toBeTruthy();
    await user.click(screen.getByRole("link", { name: "Gateway" }));
    expect(navigate).toHaveBeenCalledWith("#gateway-1");
  });

  it.each([
    { altKey: true },
    { button: 1 },
    { ctrlKey: true },
    { metaKey: true },
    { shiftKey: true },
  ])("preserves modified browser navigation for %o", (eventInit) => {
    render(
      <GatewayUiProvider gateways={gatewayOperations} navigation={navigation}>
        <TestLink />
      </GatewayUiProvider>,
    );

    screen.getByRole("link", { name: "Gateway" }).dispatchEvent(
      new MouseEvent("click", {
        bubbles: true,
        cancelable: true,
        ...eventInit,
      }),
    );

    expect(navigate).not.toHaveBeenCalled();
  });

  it("rejects consumers without a host provider", () => {
    expect(() => render(<ServicesConsumer />)).toThrow(
      "Gateway UI must be rendered within GatewayUiProvider",
    );
  });
});
