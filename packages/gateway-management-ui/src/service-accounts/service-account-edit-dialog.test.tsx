import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { IntlProvider } from "react-intl";
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  GatewayOperationError,
  type GatewayOperations,
  type OpenShellGatewayServiceAccountRecord,
} from "../application/gateway-types";
import { GatewayUiProvider } from "../gateway-ui-provider";
import { ServiceAccountEditDialog } from "./service-account-edit-dialog";

const account: OpenShellGatewayServiceAccountRecord = {
  clientId: "service-client",
  createdAt: "2026-08-21T12:00:00Z",
  createdByUserId: "user-1",
  description: "Deploys the documentation site",
  expiresAt: "2026-11-19T12:00:00Z",
  gatewayId: "gateway-1",
  id: "account-1",
  name: "deploy-bot",
  role: "openshell-user",
  status: "ready",
  subject: "service-account-subject",
  updatedAt: "2026-08-21T12:00:00Z",
};

const capabilities = {
  allowedRoles: ["openshell-user", "openshell-admin"] as const,
  canCreate: true,
  canManageAll: true,
  expirationPolicy: {
    defaultSeconds: 7_776_000,
    maximumSeconds: 31_536_000,
    minimumSeconds: 3_600,
  },
};

const mocks = vi.hoisted(() => ({
  updateRole: vi.fn(),
}));

const operations: GatewayOperations = {
  changeGatewayAccessRole: vi.fn(),
  createOpenShellGatewayServiceAccount: vi.fn(),
  grantGatewayAccess: vi.fn(),
  listGatewayAccess: vi.fn(),
  revokeGatewayAccess: vi.fn(),
  searchGatewayDirectory: vi.fn(),
  deleteOpenShellGatewayServiceAccount: vi.fn(),
  findGatewayPlacements: vi.fn(),
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
  updateOpenShellGatewayServiceAccountRole: mocks.updateRole,
};

function renderDialog() {
  const queryClient = new QueryClient({
    defaultOptions: {
      mutations: { retry: false },
      queries: { retry: false },
    },
  });
  return {
    ...render(
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
            <ServiceAccountEditDialog
              account={account}
              capabilities={capabilities}
              gatewayId="gateway-1"
              isOpen
              onClose={vi.fn()}
            />
          </GatewayUiProvider>
        </QueryClientProvider>
      </IntlProvider>,
    ),
    queryClient,
  };
}

describe("ServiceAccountEditDialog", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.updateRole.mockResolvedValue({
      ...account,
      role: "openshell-admin",
    });
  });

  // Verifies: OSG-SACC-11.06
  it("preselects the current role and submits the selected change", async () => {
    const user = userEvent.setup();
    renderDialog();

    expect(
      screen.getByRole("dialog", { name: "Edit deploy-bot" }),
    ).toBeTruthy();
    expect(screen.getByText("Deploys the documentation site")).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "OpenShell role" }));
    const adminDescription = screen.getByText(
      "Can perform OpenShell administrative operations on this gateway.",
    );
    const adminOption = adminDescription.closest("button");
    if (!adminOption) {
      throw new Error("OpenShell admin option was not rendered as a button");
    }
    await user.click(adminOption);
    await user.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() => {
      expect(mocks.updateRole).toHaveBeenCalledWith("gateway-1", "account-1", {
        role: "openshell-admin",
      });
    });
  });

  // Verifies: OSG-SACC-11.06
  it("sends a changed description with the role", async () => {
    const user = userEvent.setup();
    renderDialog();

    const description = screen.getByRole("textbox", {
      name: "Description (optional)",
    });
    await user.clear(description);
    await user.type(description, "Nightly builds");
    await user.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() => {
      expect(mocks.updateRole).toHaveBeenCalledWith("gateway-1", "account-1", {
        description: "Nightly builds",
        role: "openshell-user",
      });
    });
  });

  it("explains that already issued tokens keep the prior role", () => {
    renderDialog();

    expect(screen.getByText("When the change takes effect")).toBeTruthy();
    expect(
      screen.getByText(
        "The next token exchange reflects the new role. Access tokens issued before the change keep the prior role until they expire (at most a few minutes).",
      ),
    ).toBeTruthy();
  });

  // Verifies: OSG-SACC-11.06, OSG-SACC-18.03
  it("maps a creator-cap rejection to localized guidance", async () => {
    const user = userEvent.setup();
    mocks.updateRole.mockRejectedValue(
      new GatewayOperationError("denied", { code: "role-not-allowed" }),
    );
    renderDialog();

    await user.click(screen.getByRole("button", { name: "OpenShell role" }));
    const adminDescription = screen.getByText(
      "Can perform OpenShell administrative operations on this gateway.",
    );
    const adminOption = adminDescription.closest("button");
    if (!adminOption) {
      throw new Error("OpenShell admin option was not rendered as a button");
    }
    await user.click(adminOption);
    await user.click(screen.getByRole("button", { name: "Save changes" }));

    expect(
      await screen.findByText("The requested role is not permitted"),
    ).toBeTruthy();
    expect(
      screen.getByText(
        "A service account is capped by its creator's current gateway role, so this account cannot receive that role.",
      ),
    ).toBeTruthy();
  });
});
