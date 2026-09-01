import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { vi } from "vitest";

import {
  GatewayProfileUiProvider,
  useGatewayProfileLink,
  useGatewayProfileUi,
} from "./gateway-profile-ui-provider";

const gatewayProfileOperations = {
  createGatewayProfile: vi.fn(),
  getGatewayProfile: vi.fn(),
  listGatewayProfiles: vi.fn(),
  removeGatewayProfile: vi.fn(),
};

const navigate = vi.fn();
const navigation = {
  collectionHref: "/gateway-profiles",
  createHref: "/gateway-profiles/new",
  detailHref: (gatewayProfileId: string) =>
    `/gateway-profiles/${gatewayProfileId}`,
  navigate,
};

const gatewayProfileLabel = "Gateway profile";

function TestLink() {
  const link = useGatewayProfileLink("#profile-1");

  return <a {...link}>{gatewayProfileLabel}</a>;
}

function ServicesConsumer() {
  const services = useGatewayProfileUi();

  return <span>{services.navigation.createHref}</span>;
}

describe("GatewayProfileUiProvider", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("exposes host services and intercepts an unmodified link activation", async () => {
    const user = userEvent.setup();
    render(
      <GatewayProfileUiProvider
        gatewayProfiles={gatewayProfileOperations}
        navigation={navigation}
      >
        <ServicesConsumer />
        <TestLink />
      </GatewayProfileUiProvider>,
    );

    expect(screen.getByText("/gateway-profiles/new")).toBeTruthy();
    await user.click(screen.getByRole("link", { name: "Gateway profile" }));
    expect(navigate).toHaveBeenCalledWith("#profile-1");
  });

  it.each([
    { altKey: true },
    { button: 1 },
    { ctrlKey: true },
    { metaKey: true },
    { shiftKey: true },
  ])("preserves modified browser navigation for %o", (eventInit) => {
    render(
      <GatewayProfileUiProvider
        gatewayProfiles={gatewayProfileOperations}
        navigation={navigation}
      >
        <TestLink />
      </GatewayProfileUiProvider>,
    );

    screen.getByRole("link", { name: "Gateway profile" }).dispatchEvent(
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
      "Gateway profile UI must be rendered within GatewayProfileUiProvider",
    );
  });
});
