import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  buildGatewaySelectCommand,
  GatewayOperationError,
  toGatewayConnection,
  type GatewayOperations,
  type GatewayRecord,
} from "@openshift-online/hypershell-gateway-management-ui";
import { IntlProvider } from "react-intl";
import { MemoryRouter } from "react-router";
import {
  afterEach,
  beforeEach,
  describe,
  expect,
  it,
  vi,
  type Mock,
  type MockInstance,
} from "vitest";

import { englishMessages } from "../../i18n/catalog";

const terminal = vi.hoisted(() => {
  const entries: {
    command: string;
    finished?: number;
    id: string;
    lines: string[];
  }[] = [];
  return {
    entries,
    open: vi.fn(),
    startRunbookEntry: vi.fn((command: string) => {
      const entry = {
        command,
        id: `entry-${String(entries.length + 1)}`,
        lines: [] as string[],
        finished: undefined as number | undefined,
      };
      entries.push(entry);
      return {
        appendOutput: (lines: readonly string[]) => {
          entry.lines.push(...lines);
        },
        entryId: entry.id,
        finish: ({ exitCode }: { exitCode: number }) => {
          entry.finished = exitCode;
        },
      };
    }),
  };
});

const facts = vi.hoisted(() => ({
  pageViewed: vi.fn(),
  runbookStepRun: vi.fn(),
  signInPromptShown: vi.fn(),
}));

vi.mock("../terminal/terminal-context", () => ({
  useTerminal: () => ({
    close: vi.fn(),
    isOpen: false,
    open: terminal.open,
    startRunbookEntry: terminal.startRunbookEntry,
    toggle: vi.fn(),
  }),
}));

vi.mock("../../composition/docs-composition", () => ({
  createDocsGatewayNameGenerator: () => () => "calm-river-1a2b",
  docsFacts: facts,
  docsGatewayOperations: {},
}));

const { RunbookPage } = await import("./runbook-page");

/** Highlighted commands split into spans; match a whole code element. */
function codeContaining(text: string): HTMLElement[] {
  return screen.getAllByText(
    (_content, element) =>
      element?.tagName === "CODE" && element.textContent.includes(text),
  );
}
const { RunbookGatewayStep } = await import("./runbook-gateway-step");

const runningGateway: GatewayRecord = {
  clusterId: "cluster-1",
  externalDns: "gw-team.apps.example.com",
  gatewayVersion: "v0.0.120",
  id: "gw-1",
  name: "calm-river-1a2b",
  namespace: "openshell",
  oidcAudience: "openshell-api",
  oidcClientId: "openshell-cli",
  oidcIssuer: "https://issuer.example.com/realms/openshell",
  phase: "Running",
  status: "Ready",
};

type FakeOperations = Record<
  | "getGateway"
  | "getGatewayPlacementAvailability"
  | "getGatewayPlacements"
  | "listGateways"
  | "provisionGateway",
  Mock
>;

function fakeOperations(
  overrides: Partial<FakeOperations> = {},
): FakeOperations {
  return {
    getGateway: vi.fn(() => Promise.resolve(runningGateway)),
    getGatewayPlacementAvailability: vi.fn(() =>
      Promise.resolve({
        placements: [
          { available: false, selector: ["provider/kind"] },
          { available: true, selector: ["network/public", "provider/ibm"] },
          { available: true, selector: ["network/public", "provider/aws"] },
          { available: true, selector: ["network/vpn", "provider/aws"] },
        ],
      }),
    ),
    getGatewayPlacements: vi.fn(() => Promise.resolve([])),
    listGateways: vi.fn(() =>
      Promise.resolve({ items: [], page: 1, size: 20, total: 0 }),
    ),
    provisionGateway: vi.fn(({ name }: { name: string }) =>
      Promise.resolve({
        ...runningGateway,
        clusterId: "",
        id: "gw-1",
        name,
        phase: "Pending",
      }),
    ),
    ...overrides,
  };
}

function renderStep(
  fake: FakeOperations,
  {
    runbook = "github-jira",
    session = "signed-in",
    path = "/docs/runbooks/github-jira",
    prefillName,
  }: {
    path?: string;
    prefillName?: string;
    runbook?: "github-jira" | "gitlab-jira";
    session?: "signed-in" | "signed-out";
  } = {},
) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const onGatewayChange = vi.fn();
  const view = render(
    <IntlProvider locale="en" messages={englishMessages}>
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={[path]}>
          <ol>
            <RunbookGatewayStep
              onGatewayChange={onGatewayChange}
              operations={fake as unknown as GatewayOperations}
              {...(prefillName === undefined ? {} : { prefillName })}
              runbook={runbook}
              session={session}
            />
          </ol>
        </MemoryRouter>
      </QueryClientProvider>
    </IntlProvider>,
  );
  return { ...view, onGatewayChange, queryClient };
}

describe("runbook pages", () => {
  let fetchSpy: MockInstance<typeof fetch>;

  beforeEach(() => {
    terminal.entries.length = 0;
    terminal.open.mockClear();
    terminal.startRunbookEntry.mockClear();
    facts.runbookStepRun.mockClear();
    facts.pageViewed.mockClear();
    facts.signInPromptShown.mockClear();
    fetchSpy = vi.spyOn(globalThis, "fetch");
  });

  afterEach(() => {
    fetchSpy.mockRestore();
  });

  // Verifies: WEB-UDOC-03, WEB-UDOC-03.01
  it("renders every step for a signed-out reader with sign-in links and no API traffic", () => {
    const operations = fakeOperations();
    const queryClient = new QueryClient();
    render(
      <IntlProvider locale="en" messages={englishMessages}>
        <QueryClientProvider client={queryClient}>
          <MemoryRouter initialEntries={["/docs/runbooks/github-jira"]}>
            <RunbookPage runbook="github-jira" session="signed-out" />
          </MemoryRouter>
        </QueryClientProvider>
      </IntlProvider>,
    );

    expect(
      screen.getByRole("heading", { level: 1, name: "GitHub + Jira sandbox" }),
    ).toBeTruthy();
    const steps = within(screen.getByRole("list", { name: "Steps" }));
    expect(steps.getAllByRole("listitem")).toHaveLength(3);
    expect(
      screen
        .getByRole("link", { name: "Sign in to run this step here" })
        .getAttribute("href"),
    ).toBe("/auth/login?return_to=%2Fdocs%2Frunbooks%2Fgithub-jira");
    expect(screen.queryByRole("button", { name: "Run" })).toBeNull();
    expect(screen.getByText("Create the providers")).toBeTruthy();
    expect(operations.listGateways).not.toHaveBeenCalled();
    expect(fetchSpy).not.toHaveBeenCalled();
    expect(facts.signInPromptShown).toHaveBeenCalledWith(
      "/docs/runbooks/github-jira",
    );
    expect(facts.pageViewed).toHaveBeenCalledWith(
      "/docs/runbooks/github-jira",
      "signed-out",
    );
  });

  // Verifies: WEB-UDOC-03.02
  it("keeps the docs path and query as the sign-in return_to", () => {
    renderStep(fakeOperations(), {
      path: "/docs/runbooks/github-jira?name=team-gateway",
      session: "signed-out",
    });

    expect(
      screen
        .getByRole("link", { name: "Sign in to run this step here" })
        .getAttribute("href"),
    ).toBe(
      "/auth/login?return_to=%2Fdocs%2Frunbooks%2Fgithub-jira%3Fname%3Dteam-gateway",
    );
  });

  // Verifies: WEB-UDOC-05, WEB-UDOC-05.01
  it("prefills the gateway name without sending a create request", async () => {
    const operations = fakeOperations();
    renderStep(operations, { prefillName: "team-gateway" });

    expect(
      screen.getByLabelText<HTMLInputElement>(/New gateway name/u).value,
    ).toBe("team-gateway");
    await screen.findByText(
      "A new gateway will run on IBM Cloud with public internet access.",
    );
    expect(operations.provisionGateway).not.toHaveBeenCalled();
    expect(codeContaining("--name team-gateway").length).toBeGreaterThan(0);
  });

  // Verifies: WEB-UDOC-05.02, WEB-UDOC-06, WEB-UDOC-11.01
  it("creates the gateway on Run, follows it to Running, and demotes Run", async () => {
    const user = userEvent.setup();
    let phase = "Pending";
    const operations = fakeOperations({
      getGateway: vi.fn(() =>
        Promise.resolve({
          ...runningGateway,
          name: "team-gateway",
          phase,
        }),
      ),
    });
    const { queryClient } = renderStep(operations, {
      prefillName: "team-gateway",
    });

    const run = await screen.findByRole("button", { name: "Run" });
    await waitFor(() => {
      expect(run.getAttribute("aria-disabled") === "true").toBe(false);
    });
    run.focus();
    await user.click(run);

    await waitFor(() => {
      expect(operations.provisionGateway).toHaveBeenCalledWith({
        name: "team-gateway",
        placement: {
          excludedSelector: ["runtime/kata"],
          selector: ["network/public", "provider/ibm"],
        },
      });
    });
    expect(
      await screen.findByText(/Status: Waiting for placement/u),
    ).toBeTruthy();

    phase = "Provisioning";
    await act(async () => {
      await queryClient.invalidateQueries({
        queryKey: ["gateways", "detail", "gw-1"],
      });
    });
    expect(await screen.findByText(/Status: Provisioning/u)).toBeTruthy();

    phase = "Running";
    await act(async () => {
      await queryClient.invalidateQueries({
        queryKey: ["gateways", "detail", "gw-1"],
      });
    });
    expect(await screen.findByText(/Status: Running/u)).toBeTruthy();
    expect(
      await screen.findByText("Gateway team-gateway: Running"),
    ).toBeTruthy();
    expect(screen.getByRole("button", { name: "Re-check" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Run" })).toBeNull();

    expect(terminal.entries[0]?.command).toContain(
      "hsctl create gateway \\\n  --name team-gateway",
    );
    expect(terminal.entries[0]?.lines).toContain(
      "Gateway team-gateway created (ID gw-1).",
    );
    expect(terminal.entries[0]?.finished).toBe(0);
    expect(facts.runbookStepRun).toHaveBeenCalledWith("started", {
      action: "create",
      runbook: "github-jira",
    });
    expect(facts.runbookStepRun).toHaveBeenCalledWith("succeeded", {
      action: "create",
      runbook: "github-jira",
    });

    await user.click(screen.getByRole("button", { name: "Open in terminal" }));
    expect(terminal.open).toHaveBeenCalledWith({ entryId: "entry-1" });
  });

  // Verifies: WEB-UDOC-06.02
  it("shows the Connection tab gateway command once the gateway runs", async () => {
    const user = userEvent.setup();
    const operations = fakeOperations({
      listGateways: vi.fn(() =>
        Promise.resolve({
          items: [runningGateway],
          page: 1,
          size: 20,
          total: 1,
        }),
      ),
    });
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const { RunbookPageSurface } = await import("./runbook-page");
    function Page() {
      return (
        <RunbookPageSurface
          gateway={runningGateway}
          runbook="github-jira"
          step1={null}
        />
      );
    }
    render(
      <IntlProvider locale="en" messages={englishMessages}>
        <QueryClientProvider client={queryClient}>
          <MemoryRouter>
            <Page />
          </MemoryRouter>
        </QueryClientProvider>
      </IntlProvider>,
    );

    const expected = buildGatewaySelectCommand(
      toGatewayConnection(runningGateway),
    );
    expect(expected).toContain("openshell gateway add");
    const blocks = codeContaining("openshell gateway add");
    expect(blocks[0]?.textContent).toBe(expected);
    expect(codeContaining("-g calm-river-1a2b").length).toBeGreaterThan(0);
    expect(operations.listGateways).not.toHaveBeenCalled();
    void user;
  });

  // Verifies: WEB-UDOC-06.01
  it("disables Run and explains when no public placement is available", async () => {
    const operations = fakeOperations({
      getGatewayPlacementAvailability: vi.fn(() =>
        Promise.resolve({
          placements: [
            { available: false, selector: ["provider/kind"] },
            {
              available: false,
              reason: "no-eligible-cluster" as const,
              selector: ["network/public", "provider/ibm"],
            },
            {
              available: false,
              reason: "no-eligible-cluster" as const,
              selector: ["network/public", "provider/aws"],
            },
          ],
        }),
      ),
    });
    renderStep(operations);

    expect(
      await screen.findByText(/No public placement is currently available/u),
    ).toBeTruthy();
    expect(
      screen
        .getByRole("button", { name: "Run" })
        .getAttribute("aria-disabled") === "true",
    ).toBe(true);
    expect(operations.getGatewayPlacementAvailability).toHaveBeenCalledTimes(1);
  });

  // Verifies: WEB-UDOC-07, WEB-UDOC-07.01
  it("shows why Red Hat VPN placement is unavailable", async () => {
    const operations = fakeOperations({
      getGatewayPlacementAvailability: vi.fn(() =>
        Promise.resolve({
          placements: [
            {
              available: false,
              reason: "no-eligible-cluster" as const,
              selector: ["network/vpn", "provider/aws"],
            },
          ],
        }),
      ),
    });
    renderStep(operations, {
      path: "/docs/runbooks/gitlab-jira",
      runbook: "gitlab-jira",
    });

    expect(
      await screen.findByText(/Red Hat VPN placement is unavailable/u),
    ).toBeTruthy();
    expect(
      screen
        .getByRole("button", { name: "Run" })
        .getAttribute("aria-disabled") === "true",
    ).toBe(true);
    expect(operations.getGatewayPlacementAvailability).toHaveBeenCalledWith(
      [["network/vpn", "provider/aws"]],
      expect.anything(),
    );
  });

  // Verifies: WEB-UDOC-07
  it("offers only gateways on a Red Hat VPN cluster in the GitLab runbook", async () => {
    const operations = fakeOperations({
      getGatewayPlacements: vi.fn(() =>
        Promise.resolve([
          { capabilities: ["network/vpn"], id: "vpn-cluster", name: "vpn" },
          {
            capabilities: ["network/public"],
            id: "public-cluster",
            name: "public",
          },
        ]),
      ),
      listGateways: vi.fn(() =>
        Promise.resolve({
          items: [
            {
              ...runningGateway,
              clusterId: "vpn-cluster",
              id: "a",
              name: "vpn-gw",
            },
            {
              ...runningGateway,
              clusterId: "public-cluster",
              id: "b",
              name: "public-gw",
            },
          ],
          page: 1,
          size: 20,
          total: 2,
        }),
      ),
    });
    renderStep(operations, {
      path: "/docs/runbooks/gitlab-jira",
      runbook: "gitlab-jira",
    });

    const choice = await screen.findByRole("combobox", { name: "Gateway" });
    await waitFor(() => {
      expect(
        within(choice)
          .getAllByRole("option")
          .map((option) => option.textContent),
      ).toEqual(["Create a new gateway", "vpn-gw"]);
    });
  });

  // Verifies: WEB-UDOC-06
  it("checks an existing gateway and reports its status", async () => {
    const user = userEvent.setup();
    const operations = fakeOperations({
      listGateways: vi.fn(() =>
        Promise.resolve({
          items: [runningGateway],
          page: 1,
          size: 20,
          total: 1,
        }),
      ),
    });
    const { onGatewayChange } = renderStep(operations);

    const choice = await screen.findByRole("combobox", { name: "Gateway" });
    await waitFor(() => {
      expect(within(choice).getAllByRole("option")).toHaveLength(2);
    });
    await user.selectOptions(choice, "gw-1");
    expect(codeContaining("hsctl get gateway gw-1").length).toBeGreaterThan(0);
    await user.click(screen.getByRole("button", { name: "Run" }));

    expect(await screen.findByText(/Status: Running/u)).toBeTruthy();
    expect(operations.provisionGateway).not.toHaveBeenCalled();
    await waitFor(() => {
      expect(onGatewayChange).toHaveBeenLastCalledWith(
        expect.objectContaining({ id: "gw-1" }),
      );
    });

    await user.click(screen.getByRole("button", { name: "Re-check" }));
    await waitFor(() => {
      expect(facts.runbookStepRun).toHaveBeenCalledWith("succeeded", {
        action: "recheck",
        runbook: "github-jira",
      });
    });
  });

  // Verifies: WEB-UDOC-06
  it("ends a check of a gateway that is still provisioning and follows a new choice", async () => {
    const user = userEvent.setup();
    const provisioning = {
      ...runningGateway,
      id: "gw-2",
      name: "second-gateway",
      phase: "Provisioning",
    };
    const fake = fakeOperations({
      getGateway: vi.fn((id: string) =>
        Promise.resolve(id === "gw-2" ? provisioning : runningGateway),
      ),
      listGateways: vi.fn(() =>
        Promise.resolve({
          items: [runningGateway, provisioning],
          page: 1,
          size: 20,
          total: 2,
        }),
      ),
    });
    const { onGatewayChange } = renderStep(fake);

    const choice = await screen.findByRole("combobox", { name: "Gateway" });
    await waitFor(() => {
      expect(within(choice).getAllByRole("option")).toHaveLength(3);
    });
    await user.selectOptions(choice, "gw-2");
    await user.click(screen.getByRole("button", { name: "Run" }));

    expect(await screen.findByText(/Status: Provisioning/u)).toBeTruthy();
    // The check ends, so the reader can choose again.
    await waitFor(() => {
      expect(
        screen
          .getByRole("combobox", { name: "Gateway" })
          .hasAttribute("disabled"),
      ).toBe(false);
    });
    expect(terminal.entries.at(-1)?.finished).toBe(0);

    await user.selectOptions(
      screen.getByRole("combobox", { name: "Gateway" }),
      "gw-1",
    );
    expect(screen.queryByText(/Status: Provisioning/u)).toBeNull();
    await waitFor(() => {
      expect(onGatewayChange).toHaveBeenLastCalledWith(undefined);
    });
    await user.click(screen.getByRole("button", { name: "Run" }));
    await waitFor(() => {
      expect(onGatewayChange).toHaveBeenLastCalledWith(
        expect.objectContaining({ id: "gw-1" }),
      );
    });
  });

  // Verifies: WEB-UDOC-03, WEB-UDOC-05
  it("reports a failed create and re-reads the session after a denial", async () => {
    const user = userEvent.setup();
    const operations = fakeOperations({
      provisionGateway: vi.fn(() =>
        Promise.reject(new GatewayOperationError("denied")),
      ),
    });
    const { queryClient } = renderStep(operations, {
      prefillName: "team-gateway",
    });
    const invalidate = vi.spyOn(queryClient, "invalidateQueries");

    const run = await screen.findByRole("button", { name: "Run" });
    await waitFor(() => {
      expect(run.getAttribute("aria-disabled") === "true").toBe(false);
    });
    await user.click(run);

    expect(
      await screen.findByText("Failed: you are not allowed to do this."),
    ).toBeTruthy();
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["session"] });
    expect(terminal.entries[0]?.finished).toBe(1);
    expect(facts.runbookStepRun).toHaveBeenCalledWith("failed", {
      action: "create",
      failureKind: "denied",
      runbook: "github-jira",
    });
  });

  // Verifies: WEB-UDOC-05
  it("blocks Run for an invalid gateway name", async () => {
    const user = userEvent.setup();
    renderStep(fakeOperations(), { prefillName: "team-gateway" });

    const name = screen.getByLabelText(/New gateway name/u);
    await user.clear(name);
    await user.type(name, "Bad Name");

    expect(
      screen.getByText(
        "Use up to 63 lowercase letters, digits, and hyphens, starting with a letter.",
      ),
    ).toBeTruthy();
    expect(
      screen
        .getByRole("button", { name: "Run" })
        .getAttribute("aria-disabled") === "true",
    ).toBe(true);
  });
});
