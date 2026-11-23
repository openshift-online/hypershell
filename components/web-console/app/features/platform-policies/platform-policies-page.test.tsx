import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { IntlProvider } from "react-intl";
import { describe, expect, it, vi } from "vitest";

import { englishMessages } from "../../i18n/catalog";
import {
  platformPolicies,
  platformPolicySettings,
} from "./fixtures/platform-policy-fixtures";
import {
  PlatformPoliciesPage,
  type PlatformPoliciesPageProps,
} from "./platform-policies-page";

function renderPage(overrides: Partial<PlatformPoliciesPageProps> = {}) {
  const props: PlatformPoliciesPageProps = {
    filterText: "",
    isError: false,
    isLoading: false,
    isWriting: false,
    links: {
      createHref: "/admin/platform-policies/new",
      detailHref: (id) => `/admin/platform-policies/${id}`,
      editHref: (id) => `/admin/platform-policies/${id}/edit`,
      gatewaysHref: (id) => `/admin/platform-policies/${id}?tab=gateways`,
      onNavigate: vi.fn(),
    },
    onConfirmDialog: vi.fn(),
    onDialogChange: vi.fn(),
    onDismissWriteFailure: vi.fn(),
    onFilterTextChange: vi.fn(),
    onRetry: vi.fn(),
    onSortDirectionChange: vi.fn(),
    policies: platformPolicies("audit"),
    settings: platformPolicySettings("audit"),
    sortDirection: "asc",
    ...overrides,
  };
  render(
    <IntlProvider locale="en" messages={englishMessages}>
      <PlatformPoliciesPage {...props} />
    </IntlProvider>,
  );
  return props;
}

describe("PlatformPoliciesPage", () => {
  // Verifies: WEB-PADM-02
  it("shows each policy's mode, rollout counts, and 24-hour findings", async () => {
    const props = renderPage();
    const table = screen.getByRole("grid", { name: "Platform policies" });
    const baseline = within(table)
      .getByRole("link", { name: "sandbox-resource-baseline" })
      .closest("tr");
    expect(baseline).not.toBeNull();
    const row = within(baseline as HTMLElement);
    expect(row.getByText("Audit only")).toBeTruthy();
    expect(row.getByText("Applied on 2 of 4 gateways")).toBeTruthy();
    expect(row.getByText("1 cannot enforce and 1 pending")).toBeTruthy();
    expect(row.getByText("12")).toBeTruthy();
    expect(row.getByText("All gateways")).toBeTruthy();
    expect(row.getByText("CPU request")).toBeTruthy();

    const gpu = within(
      within(table)
        .getByRole("link", { name: "gpu-sandbox-cap" })
        .closest("tr") as HTMLElement,
    );
    expect(gpu.getByText("Enforced")).toBeTruthy();
    expect(gpu.getByText("All gateways except 2")).toBeTruthy();
    expect(gpu.getByText("Additional rule")).toBeTruthy();

    await userEvent.click(
      within(table).getByRole("link", { name: "sandbox-resource-baseline" }),
    );
    expect(props.links.onNavigate).toHaveBeenCalledWith(
      "/admin/platform-policies/policy-baseline",
    );
  });

  it("filters by name, sorts, and offers the global switch", async () => {
    const props = renderPage({ filterText: "gpu" });
    expect(
      screen.queryByRole("link", { name: "sandbox-resource-baseline" }),
    ).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Policy" }));
    expect(props.onSortDirectionChange).toHaveBeenCalledWith("desc");
    await userEvent.click(
      screen.getByRole("button", { name: "Switch to Enforced..." }),
    );
    expect(props.onDialogChange).toHaveBeenCalledWith({
      kind: "global-mode",
      to: "enforce",
    });
    expect(
      screen.getByText(
        "sandbox-resource-baseline cannot be enforced on 1 gateway",
      ),
    ).toBeTruthy();
  });

  it("shows a no-match state with a clear action", async () => {
    const props = renderPage({ filterText: "nothing" });
    await userEvent.click(screen.getByRole("button", { name: "Clear filter" }));
    expect(props.onFilterTextChange).toHaveBeenCalledWith("");
  });

  // Verifies: WEB-PADM-03
  it("shows the affected gateways and 24-hour findings before enforcing", async () => {
    const props = renderPage({
      dialog: { kind: "global-mode", to: "enforce" },
    });
    const dialog = screen.getByRole("dialog");
    expect(
      within(dialog).getByText("Enforce every policy on all 14 gateways?"),
    ).toBeTruthy();
    expect(
      within(dialog).getByText(
        "In the last 24 hours, 12 operations would have been rejected",
      ),
    ).toBeTruthy();
    expect(
      within(dialog).getByText(
        "41 operations would have received default values.",
      ),
    ).toBeTruthy();
    const impact = within(dialog).getByRole("grid", {
      name: "Impact by gateway",
    });
    expect(within(impact).getByText("ml-training-gateway")).toBeTruthy();
    expect(within(impact).getByText("Revision pending")).toBeTruthy();
    expect(
      within(impact).getByText(
        "Cannot enforce. The VM compute driver ignores per-sandbox CPU and memory.",
      ),
    ).toBeTruthy();
    expect(props.onConfirmDialog).not.toHaveBeenCalled();

    await userEvent.click(
      within(dialog).getByRole("button", { name: "Enforce on 14 gateways" }),
    );
    expect(props.onConfirmDialog).toHaveBeenCalledWith({
      kind: "global-mode",
      to: "enforce",
    });
  });

  // Verifies: WEB-PADM-03
  it("requires confirmation to return to audit only", async () => {
    const props = renderPage({
      dialog: { kind: "global-mode", to: "audit" },
      policies: platformPolicies("enforce"),
      settings: platformPolicySettings("enforce"),
    });
    const dialog = screen.getByRole("dialog");
    expect(
      within(dialog).getByText("Switch platform enforcement to Audit only?"),
    ).toBeTruthy();
    await userEvent.click(
      within(dialog).getByRole("button", { name: "Cancel" }),
    );
    expect(props.onDialogChange).toHaveBeenCalledWith(undefined);
    expect(props.onConfirmDialog).not.toHaveBeenCalled();
  });

  it("confirms a policy delete and a single-policy enforce", async () => {
    const props = renderPage({
      dialog: { kind: "delete", policyId: "policy-baseline" },
    });
    expect(
      screen.getByText(
        "The policy is removed from 4 gateways. This action cannot be undone.",
      ),
    ).toBeTruthy();
    await userEvent.click(
      screen.getByRole("button", { name: "Delete policy" }),
    );
    expect(props.onConfirmDialog).toHaveBeenCalledWith({
      kind: "delete",
      policyId: "policy-baseline",
    });
  });

  // Verifies: WEB-PADM-01
  it("presents a 403 write as a denied operation without a retry action", async () => {
    const props = renderPage({ writeFailure: "denied" });
    expect(
      screen.getByText("You are not allowed to make this change"),
    ).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Try again" })).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Close" }));
    expect(props.onDismissWriteFailure).toHaveBeenCalled();
  });

  it("renders the empty state", () => {
    renderPage({ policies: [] });
    expect(screen.getByText("No platform policies")).toBeTruthy();
    expect(
      screen.getByText("No audit-only policies. Nothing to report."),
    ).toBeTruthy();
  });

  it("renders the error state with retry", async () => {
    const props = renderPage({ isError: true });
    await userEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(props.onRetry).toHaveBeenCalled();
  });

  it("renders the loading state", () => {
    renderPage({ isLoading: true });
    expect(screen.getByRole("progressbar")).toBeTruthy();
  });
});
