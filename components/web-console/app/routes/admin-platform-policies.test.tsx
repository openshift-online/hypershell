import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { gatewayEffectivePoliciesQueryKey } from "@openshift-online/hypershell-gateway-management-ui";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { IntlProvider } from "react-intl";
import { createMemoryRouter, RouterProvider } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { PlatformPolicyOperationError } from "../application/platform-policies/platform-policy-types";
import {
  baselineFindings,
  baselinePolicy,
  platformPolicies,
  platformPolicySettings,
} from "../features/platform-policies/fixtures/platform-policy-fixtures";
import { englishMessages } from "../i18n/catalog";
import PlatformPoliciesRoute from "./admin-platform-policies";
import PlatformPolicyRoute from "./admin-platform-policy";

const { operations, getSession } = vi.hoisted(() => ({
  getSession: vi.fn(),
  operations: {
    checkAccess: vi.fn(),
    confirmModeChange: vi.fn(),
    createPolicy: vi.fn(),
    deletePolicy: vi.fn(),
    evaluatePolicy: vi.fn(),
    getPolicy: vi.fn(),
    getSettings: vi.fn(),
    listFindings: vi.fn(),
    listPolicies: vi.fn(),
    setGlobalMode: vi.fn(),
    updatePolicy: vi.fn(),
  },
}));

vi.mock("../composition/platform-policy-composition", () => ({
  platformPolicyOperations: operations,
}));
vi.mock("../composition/session-composition", () => ({
  sessionGateway: { getSession },
}));

function renderRoute(path: string) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const router = createMemoryRouter(
    [
      { element: <PlatformPoliciesRoute />, path: "/admin/platform-policies" },
      {
        element: <PlatformPolicyRoute />,
        path: "/admin/platform-policies/:policyId",
      },
    ],
    { initialEntries: [path] },
  );
  render(
    <IntlProvider locale="en" messages={englishMessages}>
      <QueryClientProvider client={queryClient}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    </IntlProvider>,
  );
  return { queryClient, router };
}

beforeEach(() => {
  vi.clearAllMocks();
  getSession.mockResolvedValue({
    authEnabled: true,
    authenticated: true,
    roles: [],
  });
  operations.checkAccess.mockResolvedValue("authorized");
  operations.listPolicies.mockResolvedValue(platformPolicies("audit"));
  operations.getSettings.mockResolvedValue(platformPolicySettings("audit"));
  operations.getPolicy.mockResolvedValue(baselinePolicy());
  operations.listFindings.mockResolvedValue({
    items: baselineFindings,
    page: 1,
    size: 20,
    total: 12,
  });
});

describe("platform policy admin routes", () => {
  // Verifies: WEB-PADM-01
  it("renders the not-authorized state, not policy data, without the role", async () => {
    operations.checkAccess.mockResolvedValue("denied");
    renderRoute("/admin/platform-policies");

    expect(
      await screen.findByText("You do not have access to platform policies"),
    ).toBeTruthy();
    expect(operations.listPolicies).not.toHaveBeenCalled();
    expect(screen.queryByText("sandbox-resource-baseline")).toBeNull();
  });

  it("offers a retry when the access check fails", async () => {
    operations.checkAccess.mockRejectedValueOnce(
      new PlatformPolicyOperationError("unavailable"),
    );
    renderRoute("/admin/platform-policies");
    await userEvent.click(
      await screen.findByRole("button", { name: "Try again" }),
    );
    expect(
      await screen.findByRole("link", { name: "sandbox-resource-baseline" }),
    ).toBeTruthy();
  });

  // Verifies: WEB-PADM-03, WEB-PADM-11
  it("applies the global mode only after confirmation and refreshes the views", async () => {
    operations.setGlobalMode.mockResolvedValue(
      platformPolicySettings("enforce"),
    );
    const { queryClient } = renderRoute("/admin/platform-policies");
    queryClient.setQueryData(gatewayEffectivePoliciesQueryKey("gw-1"), {});

    await userEvent.click(
      await screen.findByRole("button", { name: "Switch to Enforced..." }),
    );
    expect(operations.setGlobalMode).not.toHaveBeenCalled();
    const dialog = screen.getByRole("dialog");
    operations.listPolicies.mockResolvedValue(platformPolicies("enforce"));
    operations.getSettings.mockResolvedValue(platformPolicySettings("enforce"));
    await userEvent.click(
      within(dialog).getByRole("button", { name: "Enforce on 14 gateways" }),
    );

    await waitFor(() => {
      expect(operations.setGlobalMode).toHaveBeenCalledWith("enforce");
    });
    expect(operations.confirmModeChange).toHaveBeenCalledWith(
      "audit",
      "enforce",
    );
    await waitFor(() => {
      expect(screen.queryByRole("dialog")).toBeNull();
    });
    await waitFor(() => {
      expect(operations.listPolicies).toHaveBeenCalledTimes(2);
    });
    expect(
      await screen.findByRole("button", { name: "Switch to Audit only..." }),
    ).toBeTruthy();
    expect(
      queryClient.getQueryState(gatewayEffectivePoliciesQueryKey("gw-1"))
        ?.isInvalidated,
    ).toBe(true);
  });

  // Verifies: WEB-PADM-01
  it("surfaces a 403 write as denied and does not retry it", async () => {
    operations.updatePolicy.mockRejectedValue(
      new PlatformPolicyOperationError("denied"),
    );
    renderRoute("/admin/platform-policies");
    const row = (
      await screen.findByRole("link", { name: "sandbox-resource-baseline" })
    ).closest("tr") as HTMLElement;
    await userEvent.click(
      within(row).getByRole("button", { name: "Kebab toggle" }),
    );
    await userEvent.click(
      await screen.findByRole("menuitem", { name: /Start enforcing/u }),
    );
    await userEvent.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: "Start enforcing",
      }),
    );

    expect(
      await screen.findByText("You are not allowed to make this change"),
    ).toBeTruthy();
    expect(operations.updatePolicy).toHaveBeenCalledTimes(1);
    expect(operations.updatePolicy).toHaveBeenCalledWith("policy-baseline", {
      enforcementMode: "enforce",
    });
  });

  it("shows a conflicted alert for a 409 delete", async () => {
    operations.deletePolicy.mockRejectedValue(
      new PlatformPolicyOperationError("conflicted"),
    );
    renderRoute("/admin/platform-policies/policy-baseline");
    await userEvent.click(
      await screen.findByRole("button", { name: "Delete" }),
    );
    await userEvent.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: "Delete policy",
      }),
    );
    expect(
      await screen.findByText("This change conflicts with the current policy"),
    ).toBeTruthy();
  });

  // Verifies: WEB-PADM-11
  it("returns to the list after a successful delete", async () => {
    operations.deletePolicy.mockResolvedValue(undefined);
    const { router } = renderRoute("/admin/platform-policies/policy-baseline");
    await userEvent.click(
      await screen.findByRole("button", { name: "Delete" }),
    );
    await userEvent.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: "Delete policy",
      }),
    );
    await waitFor(() => {
      expect(router.state.location.pathname).toBe("/admin/platform-policies");
    });
    expect(operations.deletePolicy).toHaveBeenCalledWith("policy-baseline");
  });

  it("keeps detail tabs and findings filters in the URL", async () => {
    const { router } = renderRoute(
      "/admin/platform-policies/policy-baseline?tab=findings",
    );
    await userEvent.click(
      await screen.findByRole("button", { name: "Would add defaults" }),
    );
    await waitFor(() => {
      expect(router.state.location.search).toContain(
        "outcome=would-add-defaults",
      );
    });
    expect(operations.listFindings).toHaveBeenLastCalledWith(
      "policy-baseline",
      { outcome: "would-add-defaults", page: 1, size: 20 },
      expect.anything(),
    );
    await userEvent.click(screen.getByRole("tab", { name: "Gateways" }));
    await waitFor(() => {
      expect(router.state.location.search).toContain("tab=gateways");
    });
  });

  it("filters the list through the URL", async () => {
    const { router } = renderRoute("/admin/platform-policies");
    await userEvent.type(
      await screen.findByRole("textbox", { name: "Filter policies by name" }),
      "gpu",
    );
    await waitFor(() => {
      expect(router.state.location.search).toContain("q=gpu");
    });
    expect(
      screen.queryByRole("link", { name: "sandbox-resource-baseline" }),
    ).toBeNull();
  });
});
