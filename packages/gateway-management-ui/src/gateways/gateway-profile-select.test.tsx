import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { IntlProvider } from "react-intl";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useState } from "react";

import { GatewayUiProvider } from "../gateway-ui-provider";
import { GatewayProfileSelect } from "./gateway-profile-select";

const { findGatewayProfilesMock } = vi.hoisted(() => ({
  findGatewayProfilesMock: vi.fn(),
}));

const gatewayOperations = {
  createOpenShellGatewayServiceAccount: vi.fn(),
  deleteOpenShellGatewayServiceAccount: vi.fn(),
  findGatewayPlacements: vi.fn(),
  findGatewayProfiles: findGatewayProfilesMock,
  getGateway: vi.fn(),
  getGatewayPlacement: vi.fn(),
  getGatewayPlacements: vi.fn(),
  getOpenShellGatewayServiceAccount: vi.fn(),
  listGateways: vi.fn(),
  listOpenShellGatewayServiceAccounts: vi.fn(),
  provisionGateway: vi.fn(),
  removeGateway: vi.fn(),
  renameGateway: vi.fn(),
  revokeOpenShellGatewayServiceAccount: vi.fn(),
};

const navigation = {
  collectionHref: "/",
  createHref: "/gateways/new",
  detailHref: (gatewayId: string) => `/gateways/${gatewayId}`,
  navigate: vi.fn(),
};

function createTestQueryClient() {
  return new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
}

const onChangeMock = vi.fn();

function ProfileSelectHarness({ error }: { error?: string }) {
  const [value, setValue] = useState<string | null>(null);
  return (
    <GatewayProfileSelect
      error={error}
      isDisabled={false}
      onChange={(id) => {
        onChangeMock(id);
        setValue(id);
      }}
      value={value}
    />
  );
}

function renderSelect(error?: string) {
  return render(
    <IntlProvider locale="en">
      <QueryClientProvider client={createTestQueryClient()}>
        <GatewayUiProvider gateways={gatewayOperations} navigation={navigation}>
          <ProfileSelectHarness error={error} />
        </GatewayUiProvider>
      </QueryClientProvider>
    </IntlProvider>,
  );
}

describe("GatewayProfileSelect", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    findGatewayProfilesMock.mockResolvedValue({
      hasMore: false,
      items: [
        {
          description: "Small resource quota",
          id: "profile-small",
          name: "Small profile",
        },
      ],
    });
  });

  it("auto-selects the first profile on initial load", async () => {
    renderSelect();

    const input = screen.getByRole<HTMLInputElement>("combobox", {
      name: "Profile name",
    });
    await waitFor(() => {
      expect(input.value).toBe("Small profile");
    });
    expect(onChangeMock).toHaveBeenCalledWith("profile-small");
  });

  it("debounces the search into one API request", async () => {
    const user = userEvent.setup();
    renderSelect();
    // wait for auto-selection to complete (clear button becomes visible)
    await waitFor(() => {
      expect(
        screen.getByRole<HTMLInputElement>("combobox", { name: "Profile name" })
          .value,
      ).toBe("Small profile");
    });
    findGatewayProfilesMock.mockClear();

    await user.click(
      screen.getByRole("button", { name: "Clear gateway profile search" }),
    );
    await user.type(
      screen.getByRole("combobox", { name: "Profile name" }),
      "Small",
    );

    expect(findGatewayProfilesMock).not.toHaveBeenCalled();
    await waitFor(() => {
      expect(findGatewayProfilesMock).toHaveBeenCalledWith(
        "Small",
        expect.any(AbortSignal),
      );
    });
    expect(findGatewayProfilesMock).toHaveBeenCalledOnce();
  });

  it("shows a loading message while options are pending", async () => {
    const user = userEvent.setup();
    findGatewayProfilesMock.mockReturnValue(new Promise(() => undefined));
    renderSelect();

    const input = screen.getByRole("combobox", { name: "Profile name" });
    await user.click(input);

    expect(await screen.findByText("Loading gateway profiles")).toBeTruthy();
    expect(input.getAttribute("aria-busy")).toBe("true");
  });

  it("reports when no gateway profiles match the search", async () => {
    const user = userEvent.setup();
    findGatewayProfilesMock.mockResolvedValue({ hasMore: false, items: [] });
    renderSelect();

    const input = screen.getByRole("combobox", { name: "Profile name" });
    await user.click(input);
    await user.type(input, "Unknown");

    expect(
      await screen.findByText("No matching gateway profiles"),
    ).toBeTruthy();
  });

  it("guides the operator when more profiles are available", async () => {
    findGatewayProfilesMock.mockResolvedValue({
      hasMore: true,
      items: [
        {
          id: "profile-small",
          name: "Small profile",
        },
      ],
    });
    renderSelect();

    expect(
      await screen.findByText(
        "More gateway profiles are available. Refine your search to find a specific profile.",
      ),
    ).toBeTruthy();
  });

  it("recovers from a failed request with a retry action", async () => {
    const user = userEvent.setup();
    findGatewayProfilesMock
      .mockRejectedValueOnce(new Error("profiles unavailable"))
      .mockResolvedValue({
        hasMore: false,
        items: [
          {
            id: "profile-small",
            name: "Small profile",
          },
        ],
      });
    renderSelect();

    await user.click(
      await screen.findByRole("button", { name: "Refresh gateway profiles" }),
    );
    await user.click(screen.getByRole("combobox", { name: "Profile name" }));
    expect(await screen.findByText("Small profile")).toBeTruthy();
  });

  it("clears the current selection", async () => {
    const user = userEvent.setup();
    renderSelect();

    const input = screen.getByRole<HTMLInputElement>("combobox", {
      name: "Profile name",
    });
    await user.click(input);
    await user.click(await screen.findByText("Small profile"));
    onChangeMock.mockClear();

    await user.click(
      screen.getByRole("button", { name: "Clear gateway profile search" }),
    );

    expect(onChangeMock).toHaveBeenLastCalledWith(null);
    expect(input.value).toBe("");
  });

  it("supports keyboard selection", async () => {
    const user = userEvent.setup();
    renderSelect();

    const input = screen.getByRole<HTMLInputElement>("combobox", {
      name: "Profile name",
    });
    await user.click(input);
    await screen.findByText("Small profile");
    await user.keyboard("{ArrowDown}{Enter}");

    expect(input.value).toBe("Small profile");
    expect(onChangeMock).toHaveBeenLastCalledWith("profile-small");
  });

  it("restores the last accepted profile when Escape cancels a search", async () => {
    const user = userEvent.setup();
    renderSelect();

    const input = screen.getByRole<HTMLInputElement>("combobox", {
      name: "Profile name",
    });
    await user.click(input);
    await user.click(await screen.findByText("Small profile"));

    await user.type(input, "Uncommitted");
    await user.keyboard("{Escape}");

    expect(input.value).toBe("Small profile");
    expect(input.getAttribute("aria-expanded")).toBe("false");
  });

  it("renders a validation error", () => {
    renderSelect("This field is required.");

    expect(screen.getByText("This field is required.")).toBeTruthy();
  });
});
