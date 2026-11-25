import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { IntlProvider } from "react-intl";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import {
  afterEach,
  beforeEach,
  describe,
  expect,
  it,
  vi,
  type MockInstance,
} from "vitest";

import en from "../../../locales/en.json";
import { englishMessages } from "../../i18n/catalog";

const terminalOpen = vi.hoisted(() => vi.fn());
const facts = vi.hoisted(() => ({
  pageViewed: vi.fn(),
  runbookStepRun: vi.fn(),
  signInPromptShown: vi.fn(),
}));
const session = vi.hoisted(() => {
  const value: { data: unknown; isError: boolean; isPending: boolean } = {
    data: undefined,
    isError: false,
    isPending: false,
  };
  return { value };
});

vi.mock("../terminal/terminal-context", () => ({
  useTerminal: () => ({
    close: vi.fn(),
    isOpen: false,
    open: terminalOpen,
    startRunbookEntry: vi.fn(),
    toggle: vi.fn(),
  }),
}));
vi.mock("../../composition/docs-composition", () => ({
  createDocsGatewayNameGenerator: () => () => "calm-river-1a2b",
  docsFacts: facts,
  docsGatewayOperations: {},
}));
vi.mock("../shell/use-session", () => ({
  sessionQueryKey: ["session"],
  useSession: () => session.value,
}));

const { DocsHomePage } = await import("./docs-home-page");
const { InstallHsctlPage, InstallHsctlSurface } =
  await import("./install-hsctl-page");
const { ResourceLimitsPage } = await import("./resource-limits-page");
const { docsMessages } = await import("./docs-messages");

function LocationProbe() {
  const location = useLocation();
  const href = `${location.pathname}${location.search}`;
  return <output data-testid="location">{href}</output>;
}

function renderAt(path: string, element: ReactNode) {
  return render(
    <IntlProvider locale="en" messages={englishMessages}>
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter initialEntries={[path]}>
          <Routes>
            <Route
              element={
                <>
                  {element}
                  <LocationProbe />
                </>
              }
              path="*"
            />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>
    </IntlProvider>,
  );
}

describe("docs pages", () => {
  let fetchSpy: MockInstance<typeof fetch>;

  beforeEach(() => {
    fetchSpy = vi.spyOn(globalThis, "fetch");
    session.value = {
      data: { authEnabled: true, authenticated: false, roles: [] },
      isError: false,
      isPending: false,
    };
    facts.pageViewed.mockClear();
    terminalOpen.mockClear();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  // Verifies: WEB-UDOC-04, WEB-UDOC-04.01
  it("shows the docs home title, two task cards, and the muted help line", () => {
    renderAt("/docs", <DocsHomePage />);

    expect(
      screen.getByRole("heading", { level: 1, name: "Documentation" }),
    ).toBeTruthy();
    const github = screen.getByRole("link", { name: "GitHub + Jira sandbox" });
    const gitlab = screen.getByRole("link", { name: "GitLab + Jira sandbox" });
    expect(github.getAttribute("href")).toBe("/docs/runbooks/github-jira");
    expect(gitlab.getAttribute("href")).toBe("/docs/runbooks/gitlab-jira");
    expect(
      screen.getByText(
        "Set up a sandbox that can reach GitHub and Jira Cloud.",
      ),
    ).toBeTruthy();
    expect(
      screen.getByText(
        "Set up a sandbox that can reach internal Red Hat GitLab and Jira. Requires Red Hat VPN.",
      ),
    ).toBeTruthy();
    expect(
      screen.getByRole("link", { name: "Install hsctl" }).getAttribute("href"),
    ).toBe("/docs/cli/install-hsctl");
    expect(
      screen
        .getByRole("link", { name: "View resource limit help" })
        .getAttribute("href"),
    ).toBe("/docs/refusals/resource-limits");
    expect(screen.queryByRole("searchbox")).toBeNull();
    expect(fetchSpy).not.toHaveBeenCalled();
    expect(facts.pageViewed).toHaveBeenCalledWith("/docs", "signed-out");
  });

  // Verifies: WEB-UDOC-08, WEB-UDOC-08.01
  it("selects the detected macOS tab and records a manual Linux choice", async () => {
    const user = userEvent.setup();
    vi.spyOn(navigator, "platform", "get").mockReturnValue("MacIntel");
    vi.spyOn(navigator, "userAgent", "get").mockReturnValue(
      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)",
    );
    renderAt("/docs/cli/install-hsctl", <InstallHsctlPage />);

    expect(
      screen.getByRole("tab", { name: "macOS" }).getAttribute("aria-selected"),
    ).toBe("true");
    expect(screen.getByText(/hsctl-windows-amd64\.exe/u)).toBeTruthy();

    await user.click(screen.getByRole("tab", { name: "Linux" }));

    expect(screen.getByTestId("location").textContent).toBe(
      "/docs/cli/install-hsctl?os=linux",
    );
    expect(
      screen.getByRole("tab", { name: "Linux" }).getAttribute("aria-selected"),
    ).toBe("true");
    expect(
      screen.getByText(
        (_content, element) =>
          element?.tagName === "CODE" &&
          element.textContent.includes("sha256sum -c -"),
      ),
    ).toBeTruthy();

    await user.click(
      screen.getByRole("button", { name: "Open hsctl terminal" }),
    );
    expect(terminalOpen).toHaveBeenCalled();
  });

  // Verifies: WEB-UDOC-08
  it("lets a valid os parameter override detection", () => {
    vi.spyOn(navigator, "platform", "get").mockReturnValue("MacIntel");
    renderAt("/docs/cli/install-hsctl?os=linux", <InstallHsctlPage />);
    expect(
      screen.getByRole("tab", { name: "Linux" }).getAttribute("aria-selected"),
    ).toBe("true");
  });

  // Verifies: WEB-UDOC-08.03
  it("links to the releases page when no download location is configured", () => {
    renderAt(
      "/docs/cli/install-hsctl",
      <InstallHsctlSurface
        onOpenTerminal={vi.fn()}
        onSelectOs={vi.fn()}
        selectedOs="linux"
      />,
    );
    expect(
      screen
        .getByRole("link", { name: "HyperShell releases page" })
        .getAttribute("href"),
    ).toBe("https://github.com/openshift-online/hypershell/releases");
    expect(
      screen.getByText(
        (_content, element) =>
          element?.tagName === "CODE" &&
          element.textContent.includes("HSCTL_RELEASE"),
      ),
    ).toBeTruthy();
  });

  // Verifies: WEB-UDOC-08.02
  it("uses a configured download location", () => {
    renderAt(
      "/docs/cli/install-hsctl",
      <InstallHsctlSurface
        downloadBaseUrl="https://github.com/openshift-online/hypershell/releases/download/release-1"
        onOpenTerminal={vi.fn()}
        onSelectOs={vi.fn()}
        selectedOs="linux"
      />,
    );
    expect(
      screen.queryByRole("link", { name: "HyperShell releases page" }),
    ).toBeNull();
    expect(
      screen.getByText(
        (_content, element) =>
          element?.tagName === "CODE" &&
          element.textContent.includes(
            "HSCTL_URL=https://github.com/openshift-online/hypershell/releases/download/release-1",
          ),
      ),
    ).toBeTruthy();
  });

  // Verifies: WEB-UDOC-09, WEB-UDOC-09.01
  it("shows valid refusal context as text and prefills the exception message", () => {
    renderAt(
      "/docs/refusals/resource-limits?policy=sandbox-cpu-limit&gateway=team-a",
      <ResourceLimitsPage />,
    );

    const context = screen.getByRole("group", { name: "Refusal details" });
    expect(within(context).getByText("Rule: sandbox-cpu-limit")).toBeTruthy();
    expect(within(context).getByText("Gateway: team-a")).toBeTruthy();
    const message = screen.getByRole<HTMLTextAreaElement>("textbox", {
      name: "Copy exception request message",
    });
    expect(message.value).toContain("Rule: sandbox-cpu-limit");
    expect(message.value).toContain("Gateway: team-a");
    expect(
      screen
        .getByRole("link", { name: "the HyperShell support channel" })
        .getAttribute("href"),
    ).toBe("https://redhat.enterprise.slack.com/archives/C0BR5Q2NNJ0");
    const diagram = screen.getByRole("img", {
      name: "How a sandbox request is checked",
    });
    expect(diagram.querySelector("title")).not.toBeNull();
    expect(diagram.querySelector("desc")).not.toBeNull();
    expect(diagram.textContent).toContain("Refused (over a limit)");
    expect(fetchSpy).not.toHaveBeenCalled();
  });

  // Verifies: WEB-UDOC-09.02
  it("ignores an invalid query value everywhere on the page", () => {
    const { container } = renderAt(
      "/docs/refusals/resource-limits?policy=%3Cscript%3Ex%3C%2Fscript%3E",
      <ResourceLimitsPage />,
    );

    expect(container.textContent).not.toContain("<script>");
    expect(screen.queryByRole("group", { name: "Refusal details" })).toBeNull();
    const message = screen.getByRole<HTMLTextAreaElement>("textbox", {
      name: "Copy exception request message",
    });
    expect(message.value).toContain(
      "Rule: [rule name from the refusal message]",
    );
    expect(message.value).not.toContain("script");
  });

  // Verifies: WEB-UDOC-09.03
  it("keeps internal terms out of the refusal page messages", () => {
    const refusalIds = Object.values(docsMessages)
      .map((message) => message.id)
      .filter((id) => id.startsWith("app.docs.refusal."));
    expect(refusalIds.length).toBeGreaterThan(10);
    const catalog = en as Record<string, { defaultMessage: string }>;
    for (const id of refusalIds) {
      const text = catalog[id]?.defaultMessage ?? "";
      expect(text, id).not.toMatch(
        /interceptor|cedar|configmap|grpc|sidecar/iu,
      );
    }
  });
});
