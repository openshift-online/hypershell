import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { IntlProvider } from "react-intl";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { GatewayProfileOperationError } from "../application/gateway-profile-types";
import { GatewayProfileUiProvider } from "../gateway-profile-ui-provider";
import {
  GatewayProfilePage,
  GatewayProfilesPage,
} from "./gateway-profile-pages";

const {
  createGatewayProfileMock,
  getGatewayProfileMock,
  listGatewayProfilesMock,
  navigateMock,
  removeGatewayProfileMock,
} = vi.hoisted(() => ({
  createGatewayProfileMock: vi.fn(),
  getGatewayProfileMock: vi.fn(),
  listGatewayProfilesMock: vi.fn(),
  navigateMock: vi.fn(),
  removeGatewayProfileMock: vi.fn(),
}));

const gatewayProfileOperations = {
  createGatewayProfile: createGatewayProfileMock,
  getGatewayProfile: getGatewayProfileMock,
  listGatewayProfiles: listGatewayProfilesMock,
  removeGatewayProfile: removeGatewayProfileMock,
};

const navigation = {
  collectionHref: "/gateway-profiles",
  createHref: "/gateway-profiles/new",
  detailHref: (gatewayProfileId: string) =>
    `/gateway-profiles/${gatewayProfileId}`,
  navigate: navigateMock,
};

const smallProfile = {
  cpuLimitTotal: "8",
  cpuRequestTotal: "4",
  createdAt: "2026-08-10T14:30:00Z",
  description: "Small resource quota",
  id: "profile-small",
  memoryLimitTotal: "16Gi",
  name: "Small profile",
  podCount: 10,
  pvcCount: 5,
};

function renderPage(Page: () => React.ReactNode) {
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
          <Page />
        </GatewayProfileUiProvider>
      </QueryClientProvider>
    </IntlProvider>,
  );
}

describe("gateway profile pages", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    navigateMock.mockResolvedValue(undefined);
    removeGatewayProfileMock.mockResolvedValue(undefined);
    listGatewayProfilesMock.mockResolvedValue({
      items: [smallProfile],
      page: 1,
      size: 20,
      total: 1,
    });
    getGatewayProfileMock.mockResolvedValue(smallProfile);
  });

  it("lists gateway profiles with a detail link", async () => {
    renderPage(() => <GatewayProfilesPage />);

    expect(await screen.findByText("Small profile")).toBeTruthy();
    expect(screen.getByText("Small resource quota")).toBeTruthy();
    expect(
      screen.getByRole("link", { name: "Small profile" }).getAttribute("href"),
    ).toBe("/gateway-profiles/profile-small");
  });

  it("shows a load error when the list request fails", async () => {
    listGatewayProfilesMock.mockRejectedValue(new Error("boom"));
    renderPage(() => <GatewayProfilesPage />);

    expect(
      await screen.findByText("Gateway profiles could not be loaded"),
    ).toBeTruthy();
  });

  it("renders an empty state when there are no gateway profiles", async () => {
    listGatewayProfilesMock.mockResolvedValue({
      items: [],
      page: 1,
      size: 20,
      total: 0,
    });
    renderPage(() => <GatewayProfilesPage />);

    expect(await screen.findByText("No gateway profiles")).toBeTruthy();
  });

  it("navigates to the create page from the primary action", async () => {
    const user = userEvent.setup();
    renderPage(() => <GatewayProfilesPage />);

    await screen.findByText("Small profile");
    await user.click(
      screen.getByRole("link", { name: "Create gateway profile" }),
    );
    expect(navigateMock).toHaveBeenCalledWith("/gateway-profiles/new");
  });

  it("deletes a gateway profile from the row actions menu", async () => {
    const user = userEvent.setup();
    renderPage(() => <GatewayProfilesPage />);

    await screen.findByText("Small profile");
    await user.click(
      screen.getByRole("button", { name: "Actions for Small profile" }),
    );
    await user.click(
      screen.getByRole("menuitem", { name: "Delete gateway profile" }),
    );
    const dialog = await screen.findByRole("dialog");
    await user.click(
      within(dialog).getByRole("button", { name: "Delete gateway profile" }),
    );

    await waitFor(() => {
      expect(removeGatewayProfileMock).toHaveBeenCalledWith("profile-small");
    });
    expect(
      await screen.findByText("Gateway profile Small profile deleted"),
    ).toBeTruthy();
  });

  it("surfaces an in-use conflict when deletion is rejected", async () => {
    const user = userEvent.setup();
    removeGatewayProfileMock.mockRejectedValue(
      new GatewayProfileOperationError("conflict"),
    );
    renderPage(() => <GatewayProfilesPage />);

    await screen.findByText("Small profile");
    await user.click(
      screen.getByRole("button", { name: "Actions for Small profile" }),
    );
    await user.click(
      screen.getByRole("menuitem", { name: "Delete gateway profile" }),
    );
    const dialog = await screen.findByRole("dialog");
    await user.click(
      within(dialog).getByRole("button", { name: "Delete gateway profile" }),
    );

    expect(
      await within(dialog).findByText("Gateway profile is in use"),
    ).toBeTruthy();
  });

  it("shows a generic error when deletion fails", async () => {
    const user = userEvent.setup();
    removeGatewayProfileMock.mockRejectedValue(new Error("boom"));
    renderPage(() => <GatewayProfilesPage />);

    await screen.findByText("Small profile");
    await user.click(
      screen.getByRole("button", { name: "Actions for Small profile" }),
    );
    await user.click(
      screen.getByRole("menuitem", { name: "Delete gateway profile" }),
    );
    const dialog = await screen.findByRole("dialog");
    await user.click(
      within(dialog).getByRole("button", { name: "Delete gateway profile" }),
    );

    expect(
      await within(dialog).findByText("Gateway profile could not be deleted"),
    ).toBeTruthy();
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    await waitFor(() => {
      expect(screen.queryByRole("dialog")).toBeNull();
    });
  });

  it("renders provided gateway profiles without querying", async () => {
    renderPage(() => <GatewayProfilesPage gatewayProfiles={[smallProfile]} />);

    expect(await screen.findByText("Small profile")).toBeTruthy();
    expect(listGatewayProfilesMock).not.toHaveBeenCalled();
  });

  it("renders gateway profile details with the resource quota", () => {
    renderPage(() => (
      <GatewayProfilePage
        gatewayProfile={smallProfile}
        gatewayProfileId="profile-small"
      />
    ));

    expect(screen.getByRole("heading", { name: "Small profile" })).toBeTruthy();
    expect(screen.getByText("Total CPU request")).toBeTruthy();
    expect(screen.getByText("Small resource quota")).toBeTruthy();
  });

  it("loads gateway profile details by id", async () => {
    renderPage(() => <GatewayProfilePage gatewayProfileId="profile-small" />);

    expect(
      await screen.findByRole("heading", { name: "Small profile" }),
    ).toBeTruthy();
    expect(getGatewayProfileMock).toHaveBeenCalledWith(
      "profile-small",
      expect.any(AbortSignal),
    );
  });

  it("shows a load error on the detail page when the request fails", async () => {
    getGatewayProfileMock.mockRejectedValue(new Error("boom"));
    renderPage(() => <GatewayProfilePage gatewayProfileId="profile-small" />);

    expect(
      await screen.findByText("Gateway profiles could not be loaded"),
    ).toBeTruthy();
  });

  it("deletes from the detail page and navigates back", async () => {
    const user = userEvent.setup();
    renderPage(() => (
      <GatewayProfilePage
        gatewayProfile={smallProfile}
        gatewayProfileId="profile-small"
      />
    ));

    await user.click(
      screen.getByRole("button", { name: "Delete gateway profile" }),
    );
    const dialog = await screen.findByRole("dialog");
    await user.click(
      within(dialog).getByRole("button", { name: "Delete gateway profile" }),
    );

    await waitFor(() => {
      expect(navigateMock).toHaveBeenCalledWith("/gateway-profiles", {
        state: { deletedGatewayProfileName: "Small profile" },
      });
    });
  });
});
