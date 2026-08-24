import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { IntlProvider } from "react-intl";
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  GatewayOperationError,
  type GatewayOperations,
  type OpenShellGatewayServiceAccountRecord,
} from "../application/gateway-types";
import { GatewayUiProvider } from "../gateway-ui-provider";
import { ServiceAccountsPage } from "./service-accounts-page";

const account: OpenShellGatewayServiceAccountRecord = {
  clientId: "service-client",
  createdAt: "2026-08-21T12:00:00Z",
  createdByUserId: "user-1",
  description: "Deploys the documentation site",
  expiresAt: "2026-11-19T12:00:00Z",
  gatewayId: "gateway-1",
  id: "account-1",
  name: "deploy-bot",
  role: "openshell-admin",
  status: "ready",
  subject: "service-account-subject",
  updatedAt: "2026-08-21T12:00:00Z",
};
const connection = {
  accessTokenLifetimeSeconds: 300,
  audience: "gateway-audience",
  clientId: "service-client",
  gatewayEndpoint: "https://gateway.example.test:443",
  gatewayName: "team-gateway",
  issuer: "https://issuer.example.test/realms/openshell",
  tokenEndpoint:
    "https://issuer.example.test/realms/openshell/protocol/openid-connect/token",
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
  create: vi.fn(),
  delete: vi.fn(),
  get: vi.fn(),
  list: vi.fn(),
  revoke: vi.fn(),
}));

const operations: GatewayOperations = {
  createOpenShellGatewayServiceAccount: mocks.create,
  deleteOpenShellGatewayServiceAccount: mocks.delete,
  findGatewayPlacements: vi.fn(),
  getGateway: vi.fn(),
  getGatewayPlacement: vi.fn(),
  getGatewayPlacements: vi.fn(),
  getOpenShellGatewayServiceAccount: mocks.get,
  listGateways: vi.fn(),
  listOpenShellGatewayServiceAccounts: mocks.list,
  provisionGateway: vi.fn(),
  removeGateway: vi.fn(),
  renameGateway: vi.fn(),
  revokeOpenShellGatewayServiceAccount: mocks.revoke,
};

function page(
  items: readonly OpenShellGatewayServiceAccountRecord[] = [account],
) {
  return {
    capabilities,
    items,
    page: 1,
    size: 20,
    total: items.length,
  };
}

function renderPage() {
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
            <ServiceAccountsPage gatewayId="gateway-1" />
          </GatewayUiProvider>
        </QueryClientProvider>
      </IntlProvider>,
    ),
    queryClient,
  };
}

describe("ServiceAccountsPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.list.mockResolvedValue(page());
    mocks.get.mockResolvedValue({ connection, serviceAccount: account });
    mocks.revoke.mockResolvedValue({ ...account, status: "revoking" });
    mocks.delete.mockResolvedValue(undefined);
    mocks.create.mockResolvedValue({
      credential: { ...connection, clientSecret: "literal-client-secret" },
      serviceAccount: account,
    });
  });

  it("uses authoritative search and exposes non-secret row details", async () => {
    const user = userEvent.setup();
    renderPage();

    expect(await screen.findByText("deploy-bot")).toBeTruthy();
    await user.click(
      screen.getByRole("button", { name: "View account details" }),
    );
    expect(screen.getByText("Deploys the documentation site")).toBeTruthy();
    expect(screen.getByDisplayValue("service-account-subject")).toBeTruthy();
    expect(screen.getByText("Service account subject")).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Copy Service account subject" }),
    ).toBeTruthy();

    expect(
      screen.queryByRole("combobox", { name: "Filter by status" }),
    ).toBeNull();

    await user.type(
      screen.getByRole("textbox", { name: "Filter by name or client ID" }),
      "deploy",
    );
    await waitFor(
      () => {
        expect(mocks.list).toHaveBeenLastCalledWith(
          "gateway-1",
          expect.objectContaining({ search: "deploy" }),
          expect.any(AbortSignal),
        );
      },
      { timeout: 1_000 },
    );
  });

  it("keeps the one-time secret local while completing setup", async () => {
    const user = userEvent.setup();
    const { queryClient } = renderPage();
    await screen.findByText("deploy-bot");

    await user.click(
      screen.getByRole("button", { name: "Create service account" }),
    );
    const createDialog = screen.getByRole("dialog", {
      name: "Create service account",
    });
    await user.type(
      within(createDialog).getByRole("textbox", {
        name: "Service account name",
      }),
      "release-bot",
    );
    await user.type(
      within(createDialog).getByRole("textbox", {
        name: "Description (optional)",
      }),
      "Publishes releases",
    );
    await user.click(
      within(createDialog).getByRole("button", { name: "OpenShell role" }),
    );
    expect(
      screen.getByText(
        "Can authenticate as openshell-user. A gateway administrator must separately add its subject to each workspace it needs.",
      ),
    ).toBeTruthy();
    const adminDescription = screen.getByText(
      "Can perform OpenShell administrative operations on this gateway.",
    );
    expect(adminDescription).toBeTruthy();
    const adminOption = adminDescription.closest("button");
    if (!adminOption) {
      throw new Error("OpenShell admin option was not rendered as a button");
    }
    await user.click(adminOption);
    const expiration = within(createDialog).getByRole("button", {
      name: "Expiration",
    });
    expect(expiration.textContent).toBe("90 days");
    await user.click(expiration);
    expect(screen.getByText("30 days")).toBeTruthy();
    expect(screen.getByText("60 days")).toBeTruthy();
    expect(screen.getAllByText("90 days")).toHaveLength(2);
    await user.click(screen.getByText("60 days"));
    expect(expiration.textContent).toBe("60 days");
    await user.click(
      within(createDialog).getByRole("button", {
        name: "Create service account",
      }),
    );

    await waitFor(() => {
      expect(mocks.create).toHaveBeenCalledWith(
        "gateway-1",
        expect.objectContaining({
          description: "Publishes releases",
          name: "release-bot",
          role: "openshell-admin",
        }),
      );
    });
    const setupDialog = await screen.findByRole("dialog", {
      name: "Set up deploy-bot",
    });
    expect(within(setupDialog).queryByText("Issuer")).toBeNull();
    expect(within(setupDialog).queryByText("Token endpoint")).toBeNull();
    expect(within(setupDialog).queryByText("Gateway audience")).toBeNull();
    expect(within(setupDialog).queryByText("Gateway endpoint")).toBeNull();
    expect(
      within(setupDialog).queryByRole("button", {
        name: "Download credential bundle",
      }),
    ).toBeNull();
    const secret =
      within(setupDialog).getByLabelText<HTMLInputElement>("Client secret");
    const clientId = within(setupDialog).getByRole<HTMLInputElement>(
      "textbox",
      { name: "Client ID" },
    );
    const subject = within(setupDialog).getByRole<HTMLInputElement>("textbox", {
      name: "Service account subject",
    });
    expect(clientId.readOnly).toBe(true);
    expect(secret.readOnly).toBe(true);
    expect(subject.readOnly).toBe(true);
    expect(secret.type).toBe("password");
    expect(secret.value).toBe("literal-client-secret");
    expect(setupDialog.textContent).not.toContain("literal-client-secret");
    expect(
      JSON.stringify(
        queryClient
          .getQueryCache()
          .getAll()
          .map(({ state }) => state.data),
      ),
    ).not.toContain("literal-client-secret");
    expect(
      queryClient
        .getMutationCache()
        .getAll()
        .every(({ state }) => state.data === undefined),
    ).toBe(true);
    expect(setupDialog.textContent).toContain("openshell gateway add");
    expect(setupDialog.textContent).toContain("OPENSHELL_OIDC_CLIENT_SECRET");

    await user.click(
      within(setupDialog).getByRole("button", {
        name: "Exchange credentials for a JWT",
      }),
    );
    expect(setupDialog.textContent).toContain(
      "--data-urlencode client_secret@-",
    );
    await waitFor(() => {
      expect(setupDialog.querySelectorAll(".shiki")).toHaveLength(2);
    });
    await user.click(
      within(setupDialog).getByRole("button", { name: "Close" }),
    );
    const lossDialog = screen.getByRole("dialog", {
      name: "Leave without saving the client secret?",
    });
    expect(lossDialog.textContent).toContain("create a replacement");
    await user.click(
      within(lossDialog).getByRole("button", { name: "Return to setup" }),
    );
    await user.click(
      within(
        screen.getByRole("dialog", { name: "Set up deploy-bot" }),
      ).getByRole("checkbox", {
        name: "I saved the client secret in a secure secret manager.",
      }),
    );
    await user.click(
      within(
        screen.getByRole("dialog", { name: "Set up deploy-bot" }),
      ).getByRole("button", { name: "Finish setup" }),
    );
    await waitFor(() => {
      expect(
        screen.queryByRole("dialog", { name: "Set up deploy-bot" }),
      ).toBeNull();
    });
  });

  it("shows recovery guidance when credential copy fails", async () => {
    const user = userEvent.setup();
    vi.spyOn(window.navigator.clipboard, "writeText").mockRejectedValue(
      new Error("denied"),
    );
    renderPage();
    await screen.findByText("deploy-bot");

    await user.click(
      screen.getByRole("button", { name: "Create service account" }),
    );
    await user.type(
      screen.getByRole("textbox", { name: "Service account name" }),
      "release-bot",
    );
    await user.click(
      screen.getByRole("button", { name: "Create service account" }),
    );
    const setupDialog = await screen.findByRole("dialog", {
      name: "Set up deploy-bot",
    });

    await user.click(
      within(setupDialog).getByRole("button", { name: "Copy client secret" }),
    );
    expect(
      await within(setupDialog).findByText(
        "Could not copy to the clipboard. Copy the value manually or try again.",
      ),
    ).toBeTruthy();

    await user.click(
      within(setupDialog).getByRole("button", {
        name: "Copy OpenShell CLI setup commands",
      }),
    );
    expect(
      within(setupDialog).getAllByText(
        "Could not copy to the clipboard. Copy the value manually or try again.",
      ),
    ).toHaveLength(2);
  });

  it("keeps form input and explains an uncertain create result", async () => {
    mocks.create.mockRejectedValue(new GatewayOperationError("unavailable"));
    const user = userEvent.setup();
    renderPage();
    await screen.findByText("deploy-bot");

    await user.click(
      screen.getByRole("button", { name: "Create service account" }),
    );
    const dialog = screen.getByRole("dialog", {
      name: "Create service account",
    });
    const name = within(dialog).getByRole("textbox", {
      name: "Service account name",
    });
    await user.type(name, "release-bot");
    await user.click(
      within(dialog).getByRole("button", {
        name: "Create service account",
      }),
    );

    expect(
      await within(dialog).findByText("The create result is uncertain"),
    ).toBeTruthy();
    expect(within(dialog).getByDisplayValue("release-bot")).toBeTruthy();
    expect(dialog.textContent).toContain("delete it and create a replacement");
    expect(mocks.create).toHaveBeenCalledOnce();
  });

  it("explains how to recover when a service-account name exists", async () => {
    mocks.create.mockRejectedValue(
      new GatewayOperationError("conflict", {
        code: "service-account-name-exists",
      }),
    );
    const user = userEvent.setup();
    renderPage();
    await screen.findByText("deploy-bot");

    await user.click(
      screen.getByRole("button", { name: "Create service account" }),
    );
    const dialog = screen.getByRole("dialog", {
      name: "Create service account",
    });
    const name = within(dialog).getByRole("textbox", {
      name: "Service account name",
    });
    await user.type(name, "deploy-bot");
    await user.click(
      within(dialog).getByRole("button", {
        name: "Create service account",
      }),
    );

    expect(
      await within(dialog).findByText(
        "A service account with this name already exists",
      ),
    ).toBeTruthy();
    expect(dialog.textContent).toContain(
      "Choose a different name, or delete the active service account before reusing this name.",
    );
    expect(within(dialog).getByDisplayValue("deploy-bot")).toBeTruthy();
  });

  it("loads repeatable setup and confirms revoke and delete separately", async () => {
    const user = userEvent.setup();
    const userAccount = { ...account, role: "openshell-user" as const };
    mocks.list.mockResolvedValue(page([userAccount]));
    mocks.get.mockResolvedValue({ connection, serviceAccount: userAccount });
    renderPage();
    await screen.findByText("deploy-bot");

    const openActions = async () => {
      await user.click(
        screen.getByRole("button", {
          name: "Actions for service account deploy-bot",
        }),
      );
    };
    await openActions();
    await user.click(
      screen.getByRole("menuitem", { name: "View setup instructions" }),
    );
    const setupDialog = await screen.findByRole("dialog", {
      name: "Set up deploy-bot",
    });
    expect(within(setupDialog).getByText(/no longer available/u)).toBeTruthy();
    expect(
      within(setupDialog).getByText("Service account subject"),
    ).toBeTruthy();
    expect(setupDialog.textContent).toContain(
      "This stable ID is the JWT subject (sub).",
    );
    await user.click(
      within(setupDialog).getByRole("button", {
        name: "Grant workspace access",
      }),
    );
    expect(setupDialog.textContent).toContain("openshell workspace member add");
    expect(setupDialog.textContent).toContain(
      "--subject service-account-subject",
    );
    expect(mocks.get).toHaveBeenCalledWith(
      "gateway-1",
      "account-1",
      expect.any(AbortSignal),
    );
    await user.click(
      within(setupDialog).getByRole("button", {
        name: "Close setup instructions",
      }),
    );

    await openActions();
    await user.click(
      screen.getByRole("menuitem", { name: "Revoke service account" }),
    );
    const revokeDialog = screen.getByRole("dialog", {
      name: "Revoke deploy-bot?",
    });
    expect(revokeDialog.textContent).toContain("Tokens already issued");
    await user.click(
      within(revokeDialog).getByRole("button", {
        name: "Revoke service account",
      }),
    );
    await waitFor(() => {
      expect(mocks.revoke).toHaveBeenCalledWith("gateway-1", "account-1");
    });

    await openActions();
    await user.click(
      screen.getByRole("menuitem", { name: "Delete service account" }),
    );
    const deleteDialog = screen.getByRole("dialog", {
      name: "Delete deploy-bot?",
    });
    expect(deleteDialog.textContent).toContain("removes the Keycloak identity");
    await user.click(
      within(deleteDialog).getByRole("button", {
        name: "Delete service account",
      }),
    );
    await waitFor(() => {
      expect(mocks.delete).toHaveBeenCalledWith("gateway-1", "account-1");
    });
  });

  it("keeps the create action visible but disabled for a creator without permission", async () => {
    mocks.list.mockResolvedValue({
      ...page([]),
      capabilities: {
        ...capabilities,
        allowedRoles: ["openshell-user"],
        canCreate: false,
        canManageAll: false,
      },
    });
    renderPage();

    expect(await screen.findByText("No service accounts")).toBeTruthy();
    expect(
      screen.getByRole<HTMLButtonElement>("button", {
        name: "Create service account",
      }).disabled,
    ).toBe(true);
    expect(
      screen.getByText(
        "This list contains only service accounts that you created.",
      ),
    ).toBeTruthy();
  });

  it("arms the browser leave prompt until the secret is acknowledged", async () => {
    const addSpy = vi.spyOn(window, "addEventListener");
    const removeSpy = vi.spyOn(window, "removeEventListener");
    const user = userEvent.setup();
    renderPage();
    await screen.findByText("deploy-bot");

    await user.click(
      screen.getByRole("button", { name: "Create service account" }),
    );
    await user.type(
      screen.getByRole("textbox", { name: "Service account name" }),
      "release-bot",
    );
    await user.click(
      screen.getByRole("button", { name: "Create service account" }),
    );
    await screen.findByLabelText("Client secret");

    const registration = addSpy.mock.calls.find(
      ([type]) => type === "beforeunload",
    );
    expect(registration).toBeTruthy();
    const handler = registration?.[1] as (event: Event) => void;
    const event = new Event("beforeunload", { cancelable: true });
    const preventDefault = vi.spyOn(event, "preventDefault");
    handler(event);
    expect(preventDefault).toHaveBeenCalled();

    await user.click(
      screen.getByRole("checkbox", {
        name: "I saved the client secret in a secure secret manager.",
      }),
    );
    // Acknowledging the secret removes the risk, so the guard must disarm.
    expect(removeSpy.mock.calls.some(([type]) => type === "beforeunload")).toBe(
      true,
    );

    await user.click(screen.getByRole("button", { name: "Finish setup" }));
    await waitFor(() => {
      expect(
        screen.queryByRole("dialog", { name: "Set up deploy-bot" }),
      ).toBeNull();
    });

    addSpy.mockRestore();
    removeSpy.mockRestore();
  });

  it("arms the browser leave prompt while a create request is pending", async () => {
    let resolveCreate: (value: unknown) => void = () => undefined;
    mocks.create.mockReturnValue(
      new Promise((resolve) => {
        resolveCreate = resolve;
      }),
    );
    const addSpy = vi.spyOn(window, "addEventListener");
    const user = userEvent.setup();
    renderPage();
    await screen.findByText("deploy-bot");

    await user.click(
      screen.getByRole("button", { name: "Create service account" }),
    );
    await user.type(
      screen.getByRole("textbox", { name: "Service account name" }),
      "release-bot",
    );
    await user.click(
      screen.getByRole("button", { name: "Create service account" }),
    );

    await waitFor(() => {
      expect(addSpy.mock.calls.some(([type]) => type === "beforeunload")).toBe(
        true,
      );
    });

    resolveCreate({
      credential: { ...connection, clientSecret: "literal-client-secret" },
      serviceAccount: account,
    });
    addSpy.mockRestore();
  });
});
