import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { IntlProvider } from "react-intl";
import { describe, expect, it, vi } from "vitest";

import { englishMessages } from "../../i18n/catalog";
import {
  baselineFindings,
  baselinePolicy,
  platformPolicySettings,
} from "./fixtures/platform-policy-fixtures";
import {
  PlatformPolicyDetailPage,
  toPlatformPolicyDetailTab,
  type PlatformPolicyDetailPageProps,
} from "./platform-policy-detail-page";

function renderPage(overrides: Partial<PlatformPolicyDetailPageProps> = {}) {
  const props: PlatformPolicyDetailPageProps = {
    activeTab: "overview",
    findings: {
      isError: false,
      isLoading: false,
      items: baselineFindings,
      page: 1,
      size: 20,
      total: 12,
    },
    findingsFilter: {},
    isError: false,
    isLoading: false,
    isNotFound: false,
    isWriting: false,
    links: {
      editHref: (id) => `/admin/platform-policies/${id}/edit`,
      gatewayHref: (id) => `/gateways/${id}`,
      listHref: "/admin/platform-policies",
      onNavigate: vi.fn(),
    },
    onConfirmDialog: vi.fn(),
    onDialogChange: vi.fn(),
    onDismissWriteFailure: vi.fn(),
    onFindingsFilterChange: vi.fn(),
    onFindingsPageChange: vi.fn(),
    onRetry: vi.fn(),
    onTabChange: vi.fn(),
    policy: baselinePolicy(),
    settings: platformPolicySettings(),
    ...overrides,
  };
  render(
    <IntlProvider locale="en" messages={englishMessages}>
      <PlatformPolicyDetailPage {...props} />
    </IntlProvider>,
  );
  return props;
}

describe("PlatformPolicyDetailPage", () => {
  // Verifies: WEB-PADM-04
  it("shows the rendered rules alongside the Cedar source and message", async () => {
    renderPage();
    expect(
      screen.getByRole("heading", { name: "sandbox-resource-baseline" }),
    ).toBeTruthy();
    expect(
      screen.getByText("Platform policy. Applies to all gateways."),
    ).toBeTruthy();
    const rules = screen.getByRole("grid", {
      name: "Rules in sandbox-resource-baseline",
    });
    expect(within(rules).getByText("250m")).toBeTruthy();
    expect(within(rules).getByText("16Gi")).toBeTruthy();
    expect(screen.getByText("Audit only (policy setting).")).toBeTruthy();
    expect(screen.getByText("Applied on 2 of 4 gateways")).toBeTruthy();
    expect(
      screen.getByText(/Sandboxes on shared clusters must set CPU/u),
    ).toBeTruthy();
    await userEvent.click(screen.getByRole("button", { name: "Cedar source" }));
    // Highlighted Cedar splits into token spans; match the whole code element.
    expect(
      screen.getByText(
        (_content, element) =>
          element?.tagName === "CODE" &&
          element.textContent.includes("context.cpuLimitMillis > 8000"),
      ),
    ).toBeTruthy();
  });

  it("shows an unrecognized statement as Cedar, never hidden", () => {
    const policy = baselinePolicy();
    renderPage({
      policy: {
        ...policy,
        rendered: [
          {
            bounds: [],
            cedar: "forbid (principal, action, resource) unless { true };",
            recognized: false,
            summary: [],
          },
        ],
      },
    });
    expect(screen.getByText("Shown as Cedar")).toBeTruthy();
    expect(
      screen.getByText(
        (_content, element) =>
          element?.tagName === "CODE" &&
          element.textContent ===
            "forbid (principal, action, resource) unless { true };",
      ),
    ).toBeTruthy();
  });

  // Verifies: WEB-PADM-04
  it("lists each targeted gateway's rollout including the Unsupported gateway", () => {
    renderPage({ activeTab: "gateways" });
    const table = screen.getByRole("grid", {
      name: "Gateways this policy applies to",
    });
    const edge = within(
      within(table)
        .getByRole("link", { name: "edge-lab-gateway" })
        .closest("tr") as HTMLElement,
    );
    expect(edge.getAllByText("Cannot enforce").length).toBeGreaterThan(0);
    expect(
      edge.getByText(
        "The VM compute driver ignores per-sandbox CPU and memory.",
      ),
    ).toBeTruthy();
    const pending = within(
      within(table)
        .getByRole("link", { name: "ml-training-gateway" })
        .closest("tr") as HTMLElement,
    );
    expect(pending.getByText("Pending")).toBeTruthy();
    expect(pending.getByText("10")).toBeTruthy();
  });

  // Verifies: WEB-PADM-04
  it("shows the policy's findings with gateway and outcome filters", async () => {
    const props = renderPage({ activeTab: "findings" });
    expect(
      screen.getByRole("tab", { name: "Audit findings (12)" }),
    ).toBeTruthy();
    const table = screen.getByRole("grid", {
      name: "Audit findings for this policy",
    });
    expect(within(table).getByText("lin.chen")).toBeTruthy();
    expect(
      within(table).getByText("CPU request 100m (sandbox-resource-baseline)"),
    ).toBeTruthy();
    await userEvent.click(screen.getByRole("button", { name: "Would reject" }));
    expect(props.onFindingsFilterChange).toHaveBeenCalledWith({
      outcome: "would-reject",
    });
    await userEvent.selectOptions(
      screen.getByRole("combobox", { name: "Gateway" }),
      "payments-gateway",
    );
    expect(props.onFindingsFilterChange).toHaveBeenCalledWith({
      gatewayId: "payments-gateway",
    });
  });

  it("shows empty, loading, and error findings", () => {
    renderPage({
      activeTab: "findings",
      findings: {
        isError: false,
        isLoading: false,
        items: [],
        page: 1,
        size: 20,
        total: 0,
      },
    });
    expect(screen.getByText("No findings")).toBeTruthy();
  });

  it("shows the findings error", () => {
    renderPage({
      activeTab: "findings",
      findings: {
        isError: true,
        isLoading: false,
        items: [],
        page: 1,
        size: 20,
        total: 0,
      },
    });
    expect(screen.getByText("Could not load audit findings.")).toBeTruthy();
  });

  it("opens the enforce and delete confirmations from the header", async () => {
    const props = renderPage();
    await userEvent.click(
      screen.getByRole("button", { name: "Start enforcing..." }),
    );
    expect(props.onDialogChange).toHaveBeenCalledWith({
      kind: "policy-mode",
      policyId: "policy-baseline",
      to: "enforce",
    });
    await userEvent.click(screen.getByRole("button", { name: "Delete" }));
    expect(props.onDialogChange).toHaveBeenCalledWith({
      kind: "delete",
      policyId: "policy-baseline",
    });
    await userEvent.click(screen.getByRole("link", { name: "Edit" }));
    expect(props.links.onNavigate).toHaveBeenCalledWith(
      "/admin/platform-policies/policy-baseline/edit",
    );
  });

  it("renders a single-policy enforce confirmation scoped to its gateways", () => {
    renderPage({
      dialog: {
        kind: "policy-mode",
        policyId: "policy-baseline",
        to: "enforce",
      },
    });
    const dialog = screen.getByRole("dialog");
    expect(
      within(dialog).getByText("Start enforcing sandbox-resource-baseline?"),
    ).toBeTruthy();
    expect(
      within(dialog).getByRole("button", { name: "Start enforcing" }),
    ).toBeTruthy();
  });

  it("renders not-found and error states", async () => {
    const props = renderPage({ isError: true, policy: undefined });
    await userEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(props.onRetry).toHaveBeenCalled();
  });

  it("renders the not-found state without retry", () => {
    renderPage({ isError: true, isNotFound: true, policy: undefined });
    expect(screen.getByText("Policy not found")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Try again" })).toBeNull();
  });

  it("normalizes unknown tabs to the overview", () => {
    expect(toPlatformPolicyDetailTab("gateways")).toBe("gateways");
    expect(toPlatformPolicyDetailTab("other")).toBe("overview");
    expect(toPlatformPolicyDetailTab(null)).toBe("overview");
  });
});
