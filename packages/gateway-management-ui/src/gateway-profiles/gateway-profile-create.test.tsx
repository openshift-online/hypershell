import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { IntlProvider } from "react-intl";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { GatewayProfileUiProvider } from "../gateway-profile-ui-provider";
import { GatewayProfileCreatePage } from "./gateway-profile-create";

const { createGatewayProfileMock, navigateMock } = vi.hoisted(() => ({
  createGatewayProfileMock: vi.fn(),
  navigateMock: vi.fn(),
}));

const gatewayProfileOperations = {
  createGatewayProfile: createGatewayProfileMock,
  getGatewayProfile: vi.fn(),
  listGatewayProfiles: vi.fn(),
  removeGatewayProfile: vi.fn(),
};

const navigation = {
  collectionHref: "/gateway-profiles",
  createHref: "/gateway-profiles/new",
  detailHref: (gatewayProfileId: string) =>
    `/gateway-profiles/${gatewayProfileId}`,
  navigate: navigateMock,
};

const createdGatewayProfile = {
  id: "profile-1",
  name: "Small profile",
};

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: {
      mutations: { retry: false },
      queries: { retry: false },
    },
  });

  return render(
    <IntlProvider locale="en">
      <QueryClientProvider client={queryClient}>
        <GatewayProfileUiProvider
          gatewayProfiles={gatewayProfileOperations}
          navigation={navigation}
        >
          <GatewayProfileCreatePage />
        </GatewayProfileUiProvider>
      </QueryClientProvider>
    </IntlProvider>,
  );
}

describe("GatewayProfileCreatePage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    navigateMock.mockResolvedValue(undefined);
  });

  it("requires a name before sending a request", async () => {
    const user = userEvent.setup();
    renderPage();

    await user.click(
      screen.getByRole("button", { name: "Create gateway profile" }),
    );

    expect(await screen.findByText("This field is required.")).toBeTruthy();
    expect(createGatewayProfileMock).not.toHaveBeenCalled();
  });

  it("creates a profile with only a name and navigates to its detail page", async () => {
    const user = userEvent.setup();
    createGatewayProfileMock.mockResolvedValue(createdGatewayProfile);
    renderPage();

    await user.type(
      screen.getByRole("textbox", { name: "Profile name" }),
      "  Small profile  ",
    );
    await user.click(
      screen.getByRole("button", { name: "Create gateway profile" }),
    );

    await waitFor(() => {
      expect(createGatewayProfileMock).toHaveBeenCalledWith({
        name: "Small profile",
      });
    });
    await waitFor(() => {
      expect(navigateMock).toHaveBeenCalledWith("/gateway-profiles/profile-1");
    });
  });

  it("creates a profile with resource quota values", async () => {
    const user = userEvent.setup();
    createGatewayProfileMock.mockResolvedValue(createdGatewayProfile);
    renderPage();

    await user.type(
      screen.getByRole("textbox", { name: "Profile name" }),
      "Small profile",
    );
    await user.type(
      screen.getByRole("textbox", { name: "Total CPU request" }),
      "4",
    );
    await user.type(screen.getByRole("textbox", { name: "Pods" }), "10");
    await user.click(
      screen.getByRole("button", { name: "Create gateway profile" }),
    );

    await waitFor(() => {
      expect(createGatewayProfileMock).toHaveBeenCalledWith({
        cpuRequestTotal: "4",
        name: "Small profile",
        podCount: 10,
      });
    });
  });

  it("rejects a non-integer count", async () => {
    const user = userEvent.setup();
    renderPage();

    await user.type(
      screen.getByRole("textbox", { name: "Profile name" }),
      "Small profile",
    );
    await user.type(screen.getByRole("textbox", { name: "Pods" }), "abc");
    await user.click(
      screen.getByRole("button", { name: "Create gateway profile" }),
    );

    expect(await screen.findByText("This field is required.")).toBeTruthy();
    expect(createGatewayProfileMock).not.toHaveBeenCalled();
  });

  it("surfaces a creation error", async () => {
    const user = userEvent.setup();
    createGatewayProfileMock.mockRejectedValue(new Error("boom"));
    renderPage();

    await user.type(
      screen.getByRole("textbox", { name: "Profile name" }),
      "Small profile",
    );
    await user.click(
      screen.getByRole("button", { name: "Create gateway profile" }),
    );

    expect(
      await screen.findByText("Gateway profile could not be created"),
    ).toBeTruthy();
    expect(navigateMock).not.toHaveBeenCalled();
  });

  it("cancels back to the collection", async () => {
    const user = userEvent.setup();
    renderPage();

    await user.click(screen.getByRole("button", { name: "Cancel" }));

    expect(navigateMock).toHaveBeenCalledWith("/gateway-profiles");
  });
});
