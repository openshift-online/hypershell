import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { IntlProvider } from "react-intl";
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  type GatewayAccessCapabilities,
  type GatewayAccessGrantRecord,
  type GatewayAccessPage,
  type GatewayOperations,
} from "../application/gateway-types";
import { GatewayUiProvider } from "../gateway-ui-provider";
import { AccessPage } from "./access-page";

const creator: GatewayAccessGrantRecord = {
  grantedAt: "2026-08-21T12:00:00Z",
  isCreator: true,
  name: "Olivia Owner",
  role: "owner",
  roleBindingId: "rb-owner",
  userId: "u-owner",
  username: "owner1",
};
const secondOwner: GatewayAccessGrantRecord = {
  grantedAt: "2026-08-21T12:00:00Z",
  isCreator: false,
  name: "Owen Two",
  role: "owner",
  roleBindingId: "rb-owner-2",
  userId: "u-owner-2",
  username: "owner2",
};
const admin: GatewayAccessGrantRecord = {
  grantedAt: "2026-08-21T12:00:00Z",
  isCreator: false,
  name: "Amy Admin",
  role: "admin",
  roleBindingId: "rb-admin",
  userId: "u-admin",
  username: "admin1",
};
const user: GatewayAccessGrantRecord = {
  grantedAt: "2026-08-21T12:00:00Z",
  isCreator: false,
  name: "Uma User",
  role: "user",
  roleBindingId: "rb-user",
  userId: "u-user",
  username: "user1",
};

const ownerCapabilities: GatewayAccessCapabilities = {
  callerRole: "owner",
  canManageAccess: true,
  canManageOwners: true,
};
const adminCapabilities: GatewayAccessCapabilities = {
  callerRole: "admin",
  canManageAccess: true,
  canManageOwners: false,
};
const viewerCapabilities: GatewayAccessCapabilities = {
  callerRole: "user",
  canManageAccess: false,
  canManageOwners: false,
};

function accessPage(
  items: readonly GatewayAccessGrantRecord[],
  capabilities: GatewayAccessCapabilities,
): GatewayAccessPage {
  return { capabilities, items, page: 1, size: 20, total: items.length };
}

const mocks = vi.hoisted(() => ({
  change: vi.fn(),
  grant: vi.fn(),
  list: vi.fn(),
  revoke: vi.fn(),
  search: vi.fn(),
}));

const operations: GatewayOperations = {
  changeGatewayAccessRole: mocks.change,
  createOpenShellGatewayServiceAccount: vi.fn(),
  deleteOpenShellGatewayServiceAccount: vi.fn(),
  findGatewayPlacements: vi.fn(),
  getGateway: vi.fn(),
  getGatewayPlacement: vi.fn(),
  getGatewayPlacements: vi.fn(),
  getOpenShellGatewayServiceAccount: vi.fn(),
  grantGatewayAccess: mocks.grant,
  listGatewayAccess: mocks.list,
  listGateways: vi.fn(),
  listOpenShellGatewayServiceAccounts: vi.fn(),
  provisionGateway: vi.fn(),
  removeGateway: vi.fn(),
  renameGateway: vi.fn(),
  revokeGatewayAccess: mocks.revoke,
  revokeOpenShellGatewayServiceAccount: vi.fn(),
  searchGatewayDirectory: mocks.search,
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
        <GatewayUiProvider
          gateways={operations}
          navigation={{
            collectionHref: "/",
            createHref: "/gateways/new",
            detailHref: (id) => `/gateways/${id}`,
            navigate: vi.fn(),
          }}
        >
          <AccessPage gatewayId="gateway-1" />
        </GatewayUiProvider>
      </QueryClientProvider>
    </IntlProvider>,
  );
}

describe("AccessPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.list.mockResolvedValue(
      accessPage([creator, secondOwner, admin, user], ownerCapabilities),
    );
    mocks.search.mockResolvedValue([]);
    mocks.grant.mockResolvedValue(user);
    mocks.change.mockResolvedValue({ ...user, role: "admin" });
    mocks.revoke.mockResolvedValue(undefined);
  });

  it("renders names, user IDs, roles, and marks the creator", async () => {
    renderPage();
    expect(await screen.findByText("Olivia Owner")).toBeTruthy();
    expect(screen.getByText("owner1")).toBeTruthy();
    expect(screen.getByText("Amy Admin")).toBeTruthy();
    expect(screen.getByText("Uma User")).toBeTruthy();
    // Exactly one Creator marker.
    expect(screen.getAllByText("Creator")).toHaveLength(1);
  });

  it("disables the sole owner's controls", async () => {
    mocks.list.mockResolvedValue(accessPage([creator], ownerCapabilities));
    renderPage();
    await screen.findByText("Olivia Owner");
    // Both the inline role control and the remove action carry the reason.
    const controls = screen.getAllByRole<HTMLButtonElement>("button", {
      name: "A gateway must keep at least one owner.",
    });
    expect(controls.length).toBeGreaterThanOrEqual(1);
    controls.forEach((control) => {
      expect(control.disabled).toBe(true);
    });
  });

  it("disables owner rows for a non-owner admin", async () => {
    mocks.list.mockResolvedValue(
      accessPage([creator, user], adminCapabilities),
    );
    renderPage();
    await screen.findByText("Olivia Owner");
    // The owner row's controls are disabled with the owner-only reason.
    const ownerControls = screen.getAllByRole<HTMLButtonElement>("button", {
      name: "Only owners can change or remove an owner.",
    });
    expect(ownerControls.length).toBeGreaterThanOrEqual(1);
    ownerControls.forEach((control) => {
      expect(control.disabled).toBe(true);
    });
    // The user row remains actionable.
    expect(
      screen.getByRole<HTMLButtonElement>("button", {
        name: "Remove access: Uma User",
      }).disabled,
    ).toBe(false);
  });

  it("filters by search text and role", async () => {
    const user1 = userEvent.setup();
    renderPage();
    await screen.findByText("Olivia Owner");

    await user1.type(
      screen.getByRole("textbox", { name: "Find people..." }),
      "ali",
    );
    await waitFor(
      () => {
        expect(mocks.list).toHaveBeenLastCalledWith(
          "gateway-1",
          expect.objectContaining({ search: "ali" }),
          expect.any(AbortSignal),
        );
      },
      { timeout: 2_000 },
    );

    await user1.click(screen.getByRole("button", { name: "Filter by role" }));
    await user1.click(screen.getByRole("option", { name: "User" }));
    await waitFor(() => {
      expect(mocks.list).toHaveBeenLastCalledWith(
        "gateway-1",
        expect.objectContaining({ role: "user" }),
        expect.any(AbortSignal),
      );
    });
  });

  it("changes a role inline", async () => {
    const user1 = userEvent.setup();
    mocks.list.mockResolvedValue(accessPage([user], ownerCapabilities));
    renderPage();
    await screen.findByText("Uma User");

    await user1.click(
      screen.getByRole("button", { name: "Change role: Uma User" }),
    );
    await user1.click(screen.getByRole("option", { name: "Admin" }));
    await waitFor(() => {
      expect(mocks.change).toHaveBeenCalledWith("gateway-1", "u-user", "admin");
    });
  });

  it("removes access with confirmation", async () => {
    const user1 = userEvent.setup();
    mocks.list.mockResolvedValue(accessPage([user], ownerCapabilities));
    renderPage();
    await screen.findByText("Uma User");

    await user1.click(
      screen.getByRole("button", { name: "Remove access: Uma User" }),
    );
    const dialog = screen.getByRole("dialog", {
      name: "Remove access for Uma User?",
    });
    await user1.click(
      within(dialog).getByRole("button", { name: "Remove access" }),
    );
    await waitFor(() => {
      expect(mocks.revoke).toHaveBeenCalledWith("gateway-1", "u-user");
    });
  });

  it("adds a directory user with the owner-only Owner option", async () => {
    const user1 = userEvent.setup();
    mocks.search.mockResolvedValue([{ name: "Dana Scully", username: "dana" }]);
    renderPage();
    await screen.findByText("Olivia Owner");

    await user1.click(screen.getByRole("button", { name: "Add users" }));
    const dialog = screen.getByRole("dialog", { name: "Add users" });
    await user1.click(
      within(dialog).getByRole("combobox", {
        name: "Search the identity directory",
      }),
    );
    await user1.click(await screen.findByText(/Dana Scully/u));

    // Owner is offered to owner callers.
    expect(within(dialog).getByRole("radio", { name: /Owner/u })).toBeTruthy();
    await user1.click(within(dialog).getByRole("radio", { name: /Owner/u }));
    await user1.click(
      within(dialog).getByRole("button", { name: "Grant access" }),
    );
    await waitFor(() => {
      expect(mocks.grant).toHaveBeenCalledWith("gateway-1", {
        role: "owner",
        username: "dana",
      });
    });
  });

  it("hides the Owner option from a non-owner admin", async () => {
    const user1 = userEvent.setup();
    mocks.list.mockResolvedValue(accessPage([admin], adminCapabilities));
    mocks.search.mockResolvedValue([{ name: "Dee", username: "dee" }]);
    renderPage();
    await screen.findByText("Amy Admin");

    await user1.click(screen.getByRole("button", { name: "Add users" }));
    const dialog = screen.getByRole("dialog", { name: "Add users" });
    await user1.click(
      within(dialog).getByRole("combobox", {
        name: "Search the identity directory",
      }),
    );
    await user1.click(await screen.findByText(/Dee/u));
    expect(within(dialog).queryByRole("radio", { name: /Owner/u })).toBeNull();
    expect(within(dialog).getByRole("radio", { name: /Admin/u })).toBeTruthy();
    expect(within(dialog).getByRole("radio", { name: /User/u })).toBeTruthy();
  });

  it("shows viewers a read-only list", async () => {
    mocks.list.mockResolvedValue(
      accessPage([creator, user], viewerCapabilities),
    );
    renderPage();
    await screen.findByText("Olivia Owner");

    expect(
      screen.getByText("You have read-only access to this list."),
    ).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Add users" })).toBeNull();
    expect(
      screen.queryByRole("button", { name: "Remove access: Uma User" }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", { name: "Change role: Uma User" }),
    ).toBeNull();
    // Role shows as plain text for viewers.
    expect(screen.getByText("User")).toBeTruthy();
  });
});
