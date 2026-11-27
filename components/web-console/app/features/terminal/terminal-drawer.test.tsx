import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useState, type ReactNode } from "react";
import { IntlProvider } from "react-intl";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { TerminalLayoutPreferences } from "../../application/terminal/terminal-layout";
import {
  createFakeTerminal,
  createMemoryTerminalPreferences,
  type FakeRuntimeScript,
} from "../../application/terminal/testing/fake-terminal";
import { englishMessages } from "../../i18n/catalog";
import {
  TerminalProvider,
  useTerminal,
  type RunbookEntryHandle,
  type TerminalSessionState,
} from "./terminal-context";
import { TerminalDrawer } from "./terminal-drawer";
import { TerminalTrigger } from "./terminal-trigger";

const { getSessionMock } = vi.hoisted(() => ({ getSessionMock: vi.fn() }));

vi.mock("../../composition/session-composition", () => ({
  sessionGateway: { getSession: getSessionMock },
}));

vi.mock("../../composition/terminal-composition", () => ({
  terminalOperations: {},
  terminalPreferences: {},
}));

const signedIn: TerminalSessionState = { identity: "ada", status: "signed-in" };
const location = "/docs/runbooks/github-jira?step=1";

function setMedia({
  reducedMotion = false,
  wide = false,
}: {
  reducedMotion?: boolean;
  wide?: boolean;
}) {
  const matchMedia = vi.fn().mockImplementation((query: string) => ({
    addEventListener: vi.fn(),
    addListener: vi.fn(),
    dispatchEvent: vi.fn(),
    matches: query.includes("min-width")
      ? wide
      : query.includes("reduce")
        ? reducedMotion
        : false,
    media: query,
    onchange: null,
    removeEventListener: vi.fn(),
    removeListener: vi.fn(),
  }));
  Object.defineProperty(window, "matchMedia", {
    configurable: true,
    value: matchMedia,
  });
}

let runbook: RunbookEntryHandle | null = null;
let scrollTo = vi.fn();
const runStepLabel = "Run step";
const openInTerminalLabel = "Open in terminal";
const gatewayListLabel = "Gateway list";

function storageText(storage: Storage): string {
  return Array.from({ length: storage.length }, (_value, index) => {
    const key = storage.key(index) ?? "";
    return `${key}=${storage.getItem(key) ?? ""}`;
  }).join("\n");
}

function RunbookCell() {
  const terminal = useTerminal();
  const [entry, setEntry] = useState<string | null>(null);
  return (
    <>
      <button
        onClick={() => {
          runbook = terminal.startRunbookEntry("hsctl create gateway --name a");
          setEntry(runbook.entryId);
        }}
        type="button"
      >
        {runStepLabel}
      </button>
      <button
        onClick={() => {
          terminal.open(entry === null ? undefined : { entryId: entry });
        }}
        type="button"
      >
        {openInTerminalLabel}
      </button>
      <span data-testid="terminal-open">{String(terminal.isOpen)}</span>
    </>
  );
}

function renderTerminal({
  children,
  lineLimit,
  preferences,
  script,
  session = signedIn,
}: {
  children?: ReactNode;
  lineLimit?: number;
  preferences?: TerminalLayoutPreferences;
  script?: FakeRuntimeScript;
  session?: TerminalSessionState;
} = {}) {
  const terminal = createFakeTerminal(script);
  const store = createMemoryTerminalPreferences(preferences);
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const invalidate = vi.spyOn(queryClient, "invalidateQueries");
  const tree = (current: TerminalSessionState) => (
    <IntlProvider locale="en" messages={englishMessages}>
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={[location]}>
          <TerminalProvider
            operations={terminal.operations}
            preferences={store}
            session={current}
            {...(lineLimit === undefined ? {} : { lineLimit })}
          >
            <TerminalTrigger />
            <TerminalDrawer>
              <a href="/gateways">{gatewayListLabel}</a>
              <RunbookCell />
              {children}
            </TerminalDrawer>
          </TerminalProvider>
        </MemoryRouter>
      </QueryClientProvider>
    </IntlProvider>
  );
  const view = render(tree(session));
  return {
    ...view,
    invalidate,
    preferences: store,
    rerenderSession: (next: TerminalSessionState) => {
      view.rerender(tree(next));
    },
    terminal,
  };
}

async function openTerminal(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole("button", { name: "Open hsctl terminal" }));
  const input = screen.getByRole("textbox", { name: "hsctl command" });
  await waitFor(() => {
    expect((input as HTMLInputElement).readOnly).toBe(false);
  });
  return input as HTMLInputElement;
}

function log() {
  return screen.getByRole("log", { name: "hsctl output" });
}

beforeEach(() => {
  setMedia({});
  runbook = null;
  scrollTo = vi.fn();
  HTMLElement.prototype.scrollTo = scrollTo;
  localStorage.clear();
  sessionStorage.clear();
  getSessionMock.mockResolvedValue({
    authEnabled: false,
    authenticated: false,
    roles: [],
  });
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("terminal drawer", () => {
  // Verifies: WEB-TERM-01.01
  it("toggles with Ctrl+` and returns focus on Esc", async () => {
    const user = userEvent.setup();
    const { terminal } = renderTerminal();
    const link = screen.getByRole("link", { name: "Gateway list" });
    link.focus();

    await user.keyboard("{Control>}`{/Control}");

    const region = screen.getByRole("region", { name: "hsctl terminal" });
    const input = within(region).getByRole("textbox", {
      name: "hsctl command",
    });
    expect(document.activeElement).toBe(input);
    expect(
      screen.getByRole("button", { name: "Close hsctl terminal" }),
    ).toBeTruthy();

    await user.keyboard("{Escape}");

    expect(screen.queryByRole("region", { name: "hsctl terminal" })).toBeNull();
    expect(document.activeElement).toBe(link);
    expect(terminal.probes.map((probe) => probe.name)).toEqual([
      "terminal.drawer.opened",
      "terminal.runtime-load.started",
      "terminal.runtime-load.completed",
      "terminal.drawer.closed",
    ]);
  });

  // Verifies: WEB-TERM-01
  it("renders only the trigger while closed and returns focus to it", async () => {
    const user = userEvent.setup();
    const { terminal } = renderTerminal();
    expect(screen.queryByRole("region")).toBeNull();
    expect(screen.queryByRole("textbox")).toBeNull();
    expect(terminal.runtime.loadCount).toBe(0);

    await openTerminal(user);
    await user.click(screen.getByRole("button", { name: "Close terminal" }));

    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "Open hsctl terminal" }),
    );
    await user.keyboard("{Control>}`{/Control}");
    await user.keyboard("{Control>}`{/Control}");
    expect(screen.queryByRole("region")).toBeNull();
  });

  // Verifies: WEB-TERM-01.02
  it("shows only a sign-in link to a signed-out reader and loads nothing", async () => {
    const user = userEvent.setup();
    const { terminal } = renderTerminal({ session: { status: "signed-out" } });

    await user.click(
      screen.getByRole("button", { name: "Open hsctl terminal" }),
    );

    const region = screen.getByRole("region", { name: "hsctl terminal" });
    const signIn = within(region).getByRole("link", {
      name: "Sign in to use the hsctl terminal",
    });
    expect(signIn.getAttribute("href")).toBe(
      "/auth/login?return_to=%2Fdocs%2Frunbooks%2Fgithub-jira%3Fstep%3D1",
    );
    expect(document.activeElement).toBe(signIn);
    expect(within(region).queryByRole("textbox")).toBeNull();
    expect(within(region).queryByRole("separator")).toBeNull();
    expect(terminal.runtime.loadCount).toBe(0);
    expect(terminal.probes[0]?.fields).toEqual({ signedIn: false });
  });

  // Verifies: WEB-TERM-01.02
  it("treats an unreadable session as signed out", async () => {
    getSessionMock.mockRejectedValue(new Error("session unavailable"));
    const user = userEvent.setup();
    const terminal = createFakeTerminal();
    render(
      <IntlProvider locale="en" messages={englishMessages}>
        <QueryClientProvider
          client={
            new QueryClient({ defaultOptions: { queries: { retry: false } } })
          }
        >
          <MemoryRouter initialEntries={[location]}>
            <TerminalProvider
              operations={terminal.operations}
              preferences={createMemoryTerminalPreferences()}
            >
              <TerminalTrigger />
              <TerminalDrawer>
                <RunbookCell />
              </TerminalDrawer>
            </TerminalProvider>
          </MemoryRouter>
        </QueryClientProvider>
      </IntlProvider>,
    );

    await user.click(
      screen.getByRole("button", { name: "Open hsctl terminal" }),
    );

    expect(
      await screen.findByRole("link", {
        name: "Sign in to use the hsctl terminal",
      }),
    ).toBeTruthy();
    expect(screen.queryByText("Checking your console session")).toBeNull();
    expect(terminal.runtime.loadCount).toBe(0);
  });

  // Verifies: WEB-TERM-01
  it("waits for the session before loading the runtime", async () => {
    const user = userEvent.setup();
    const view = renderTerminal({ session: { status: "unknown" } });
    await user.click(
      screen.getByRole("button", { name: "Open hsctl terminal" }),
    );
    expect(screen.getByText("Checking your console session")).toBeTruthy();
    expect(view.terminal.runtime.loadCount).toBe(0);

    view.rerenderSession(signedIn);

    await waitFor(() => {
      expect(view.terminal.runtime.loadCount).toBe(1);
    });
  });

  // Verifies: WEB-TERM-04.01
  it("runs an hsctl command through the bridge and shows its output", async () => {
    const user = userEvent.setup();
    const { terminal } = renderTerminal({
      script: {
        respond: () => ({
          exitCode: 0,
          mutated: false,
          stderr: "",
          stdout: "NAME   STATUS\nprod   Ready\n",
        }),
      },
    });
    const input = await openTerminal(user);

    await user.type(input, "hsctl list gateways{Enter}");

    await screen.findByText(/prod\s+Ready/u);
    expect(terminal.runtime.calls).toEqual([["list", "gateways"]]);
    expect(input.value).toBe("");
    expect(within(log()).getByText("hsctl list gateways")).toBeTruthy();
    await waitFor(() => {
      expect(
        screen.getByText("Command finished with exit code 0."),
      ).toBeTruthy();
    });
  });

  // Verifies: WEB-TERM-04.02
  it("refuses shell operators without calling the bridge", async () => {
    const user = userEvent.setup();
    const { terminal } = renderTerminal();
    const input = await openTerminal(user);

    await user.type(input, "hsctl list gateways | grep prod{Enter}");

    expect(screen.getByText(/Shell operators are not supported/u)).toBeTruthy();
    expect(terminal.runtime.calls).toEqual([]);
    expect(terminal.probes.at(-1)?.fields).toEqual({
      reason: "shell-operator",
    });
  });

  // Verifies: WEB-TERM-04
  it("explains other programs, incomplete lines, and clear", async () => {
    const user = userEvent.setup();
    const { terminal } = renderTerminal();
    const input = await openTerminal(user);

    await user.type(input, "kubectl get pods{Enter}");
    expect(
      screen.getByText(
        "Only hsctl commands run here. Run kubectl and other programs, such as openshell or kubectl, in a local terminal.",
      ),
    ).toBeTruthy();
    await user.type(input, "get 'x{Enter}");
    expect(
      screen.getByText(
        "The command has an unclosed quote or a trailing backslash.",
      ),
    ).toBeTruthy();
    await user.type(input, "clear{Enter}");
    expect(within(log()).queryByText("kubectl get pods")).toBeNull();
    expect(terminal.runtime.calls).toEqual([]);
  });

  // Verifies: WEB-TERM-12.01
  it("renders markup in output as text", async () => {
    const user = userEvent.setup();
    renderTerminal({
      script: {
        respond: () => ({
          exitCode: 0,
          mutated: false,
          stderr: "",
          stdout: "<img src=x onerror=alert(1)>\n",
        }),
      },
    });
    const input = await openTerminal(user);

    await user.type(input, "version{Enter}");

    expect(
      await within(log()).findByText("<img src=x onerror=alert(1)>"),
    ).toBeTruthy();
    expect(log().querySelector("img")).toBeNull();
  });

  // Verifies: WEB-TERM-04
  it("marks standard error with a text cue and shows a non-zero exit code", async () => {
    const user = userEvent.setup();
    renderTerminal({
      script: {
        respond: () => ({
          exitCode: 2,
          mutated: false,
          stderr: "Error: unknown flag\n",
          stdout: "",
        }),
      },
    });
    const input = await openTerminal(user);

    await user.type(input, "list gateways --bogus{Enter}");

    const line = await within(log()).findByText(/Error: unknown flag/u);
    expect(line.textContent).toBe("Error output:Error: unknown flag");
    expect(within(log()).getByText("Exit code 2")).toBeTruthy();
    expect(screen.getByText("Command finished with exit code 2.")).toBeTruthy();
  });

  // Verifies: WEB-TERM-05.01
  it("cancels a running command with Ctrl+C and accepts the next one", async () => {
    const user = userEvent.setup();
    let calls = 0;
    const { invalidate, terminal } = renderTerminal({
      script: {
        respond: () => {
          calls += 1;
          return calls === 1
            ? "forever"
            : { exitCode: 0, mutated: false, stderr: "", stdout: "v1\n" };
        },
      },
    });
    const input = await openTerminal(user);

    await user.type(input, "list gateways{Enter}");
    expect(screen.getByText("Running. Press Ctrl+C to cancel.")).toBeTruthy();
    await user.type(input, "version{Enter}");
    expect(terminal.runtime.calls).toHaveLength(1);
    expect(input.value).toBe("version");

    await user.keyboard("{Control>}c{/Control}");

    expect(await within(log()).findByText("Command cancelled.")).toBeTruthy();
    expect(terminal.runtime.cancelCount).toBe(1);
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["gateways"] });
    await user.type(input, "{Enter}");
    expect(await within(log()).findByText("v1")).toBeTruthy();
  });

  // Verifies: WEB-TERM-05
  it("prints the line and ^C for Ctrl+C with no running command", async () => {
    const user = userEvent.setup();
    renderTerminal();
    const input = await openTerminal(user);

    await user.type(input, "list gate");
    await user.keyboard("{Control>}c{/Control}");

    expect(input.value).toBe("");
    expect(within(log()).getByText("list gate")).toBeTruthy();
    expect(within(log()).getByText("^C")).toBeTruthy();
  });

  // Verifies: WEB-TERM-05
  it("handles the line-editing keys while the input has focus", async () => {
    const user = userEvent.setup();
    renderTerminal();
    const input = await openTerminal(user);

    await user.type(input, "list gateways");
    await user.keyboard("{Control>}a{/Control}");
    expect(input.selectionStart).toBe(0);
    await user.keyboard("{Control>}k{/Control}");
    expect(input.value).toBe("");

    await user.type(input, "get gateway g1");
    await user.keyboard("{Control>}w{/Control}");
    expect(input.value).toBe("get gateway ");
    await user.keyboard("{Alt>}{Backspace}{/Alt}");
    expect(input.value).toBe("get ");
    await user.type(input, "gateway");
    await user.keyboard("{Control>}{Backspace}{/Control}");
    expect(input.value).toBe("get ");
    await user.keyboard("{Control>}e{/Control}");
    expect(input.selectionStart).toBe(4);
    await user.keyboard("{Control>}u{/Control}");
    expect(input.value).toBe("");

    await user.type(input, "version{Enter}");
    await user.type(input, "list gateways{Enter}");
    await user.type(input, "draft");
    await user.keyboard("{ArrowUp}");
    expect(input.value).toBe("list gateways");
    await user.keyboard("{ArrowUp}");
    expect(input.value).toBe("version");
    await user.keyboard("{ArrowDown}{ArrowDown}");
    expect(input.value).toBe("draft");

    await user.keyboard("{Control>}l{/Control}");
    expect(within(log()).queryByText("version")).toBeNull();
  });

  // Verifies: WEB-TERM-07.01
  it("invalidates gateway queries after a state-changing command", async () => {
    const user = userEvent.setup();
    const { invalidate } = renderTerminal({
      script: {
        respond: (argv) => ({
          exitCode: 0,
          mutated: argv[0] === "delete",
          stderr: "",
          stdout: "",
        }),
      },
    });
    const input = await openTerminal(user);

    await user.type(input, "list gateways{Enter}");
    await screen.findByText("Command finished with exit code 0.");
    expect(invalidate).not.toHaveBeenCalled();

    await user.type(input, "hsctl delete gateway g1 --yes{Enter}");
    await waitFor(() => {
      expect(invalidate).toHaveBeenCalledWith({ queryKey: ["gateways"] });
    });
  });

  // Verifies: WEB-TERM-10.02
  it("prints a sign-in link when the session expired and stays on the page", async () => {
    const user = userEvent.setup();
    const { terminal } = renderTerminal({
      script: {
        respond: () => ({
          exitCode: 1,
          mutated: false,
          stderr: "Error: the console session expired\n",
          stdout: "",
        }),
      },
    });
    terminal.sessionExpired.value = true;
    const input = await openTerminal(user);

    await user.type(input, "hsctl list gateways{Enter}");

    const signIn = await within(log()).findByRole("link", {
      name: "Sign in again",
    });
    expect(signIn.getAttribute("href")).toBe(
      "/auth/login?return_to=%2Fdocs%2Frunbooks%2Fgithub-jira%3Fstep%3D1",
    );
    expect(screen.getByRole("region", { name: "hsctl terminal" })).toBeTruthy();
  });

  // Verifies: WEB-TERM-06.01
  it("registers the beforeunload guard only while needed", async () => {
    const user = userEvent.setup();
    renderTerminal();
    const fire = () => {
      const event = new Event("beforeunload", { cancelable: true });
      window.dispatchEvent(event);
      return event.defaultPrevented;
    };
    const input = await openTerminal(user);
    act(() => {
      input.blur();
    });
    expect(fire()).toBe(false);

    await user.type(input, "list");
    expect(fire()).toBe(true);
    await user.clear(input);
    act(() => {
      input.blur();
    });
    expect(fire()).toBe(false);
  });

  // Verifies: WEB-TERM-03.01
  it("shrinks a 190 px drawer to 180 px with the keyboard, not 166", async () => {
    const user = userEvent.setup();
    const { preferences } = renderTerminal({
      preferences: { dock: "bottom", sizes: { bottom: 190, right: 480 } },
    });
    await openTerminal(user);
    const handle = screen.getByRole("separator", {
      name: "Resize hsctl terminal",
    });
    expect(handle.getAttribute("aria-orientation")).toBe("horizontal");
    expect(handle.getAttribute("aria-valuenow")).toBe("190");
    expect(handle.getAttribute("aria-valuemin")).toBe("180");

    handle.focus();
    await user.keyboard("{ArrowDown}");

    expect(handle.getAttribute("aria-valuenow")).toBe("180");
    expect(preferences.saved.at(-1)?.sizes.bottom).toBe(180);
    await user.keyboard("{ArrowUp}{ArrowUp}");
    expect(handle.getAttribute("aria-valuenow")).toBe("228");
    await user.keyboard("{End}");
    expect(handle.getAttribute("aria-valuenow")).toBe(
      handle.getAttribute("aria-valuemax"),
    );
    await user.keyboard("{Home}");
    expect(handle.getAttribute("aria-valuenow")).toBe("180");
  });

  // Verifies: WEB-TERM-03
  it("resizes with a pointer drag and saves on release", async () => {
    const user = userEvent.setup();
    const { preferences } = renderTerminal();
    await openTerminal(user);
    const handle = screen.getByRole("separator");

    fireEvent.pointerDown(handle, { button: 0, clientY: 500, pointerId: 1 });
    fireEvent.pointerMove(handle, { clientY: 440, pointerId: 1 });
    expect(handle.getAttribute("aria-valuenow")).toBe("380");
    fireEvent.pointerUp(handle, { clientY: 400, pointerId: 1 });

    expect(handle.getAttribute("aria-valuenow")).toBe("420");
    expect(preferences.saved.at(-1)?.sizes.bottom).toBe(420);

    // Other buttons and other pointers are ignored; a cancelled drag ends.
    const saves = preferences.saved.length;
    fireEvent.pointerDown(handle, { button: 2, clientY: 500, pointerId: 2 });
    fireEvent.pointerMove(handle, { clientY: 300, pointerId: 2 });
    fireEvent.pointerUp(handle, { clientY: 300, pointerId: 2 });
    expect(handle.getAttribute("aria-valuenow")).toBe("420");
    expect(preferences.saved).toHaveLength(saves);
    fireEvent.pointerDown(handle, { button: 0, clientY: 500, pointerId: 3 });
    fireEvent.pointerMove(handle, { clientY: 480, pointerId: 4 });
    expect(handle.getAttribute("aria-valuenow")).toBe("420");
    fireEvent.pointerCancel(handle, { clientY: 490, pointerId: 3 });
    expect(handle.getAttribute("aria-valuenow")).toBe("430");
    fireEvent.lostPointerCapture(handle, { pointerId: 3 });
    fireEvent.pointerMove(handle, { clientY: 100, pointerId: 3 });
    expect(handle.getAttribute("aria-valuenow")).toBe("430");
  });

  // Verifies: WEB-TERM-02.01
  it("docks right on a wide viewport and remembers the position", async () => {
    setMedia({ wide: true });
    const user = userEvent.setup();
    const { preferences } = renderTerminal();
    await openTerminal(user);

    await user.click(screen.getByRole("button", { name: "Dock right" }));

    expect(preferences.saved.at(-1)?.dock).toBe("right");
    const handle = screen.getByRole("separator");
    expect(handle.getAttribute("aria-orientation")).toBe("vertical");
    expect(handle.getAttribute("aria-valuemin")).toBe("320");
    handle.focus();
    await user.keyboard("{ArrowLeft}");
    expect(handle.getAttribute("aria-valuenow")).toBe("504");
    expect(screen.getByRole("button", { name: "Dock bottom" })).toBeTruthy();
  });

  // Verifies: WEB-TERM-02
  it("docks at the bottom below the md breakpoint whatever was saved", async () => {
    const user = userEvent.setup();
    renderTerminal({
      preferences: { dock: "right", sizes: { bottom: 300, right: 600 } },
    });
    await openTerminal(user);
    expect(screen.queryByRole("button", { name: /Dock/u })).toBeNull();
    expect(screen.getByRole("separator").getAttribute("aria-orientation")).toBe(
      "horizontal",
    );
  });

  // Verifies: WEB-TERM-09
  it("shows truthful progress and keeps the input from running commands while loading", async () => {
    const user = userEvent.setup();
    renderTerminal({
      script: {
        loadForever: true,
        progress: { loadedBytes: 2_000_000, totalBytes: 4_000_000 },
      },
    });

    await user.click(
      screen.getByRole("button", { name: "Open hsctl terminal" }),
    );

    expect(screen.getByRole("progressbar")).toBeTruthy();
    const input = screen.getByRole("textbox", { name: "hsctl command" });
    expect((input as HTMLInputElement).readOnly).toBe(true);
    expect(document.activeElement).toBe(input);
  });

  // Verifies: WEB-TERM-09
  it("shows downloaded bytes when the total size is unknown", async () => {
    const user = userEvent.setup();
    renderTerminal({
      script: {
        loadForever: true,
        progress: { loadedBytes: 2_400_000, totalBytes: null },
      },
    });
    await user.click(
      screen.getByRole("button", { name: "Open hsctl terminal" }),
    );
    expect(screen.getByText(/Loading hsctl: 2\.4 MB downloaded/u)).toBeTruthy();
  });

  // Verifies: WEB-TERM-09
  it("shows a localized load error with Retry", async () => {
    const user = userEvent.setup();
    const { terminal } = renderTerminal({ script: { loadFailure: "crashed" } });
    await user.click(
      screen.getByRole("button", { name: "Open hsctl terminal" }),
    );

    expect(await screen.findByText("hsctl could not be loaded")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(terminal.runtime.loadCount).toBe(2);
  });

  // Verifies: WEB-TERM-09
  it("explains a build without the runtime and offers no retry", async () => {
    const user = userEvent.setup();
    renderTerminal({ script: { loadFailure: "unavailable" } });
    await user.click(
      screen.getByRole("button", { name: "Open hsctl terminal" }),
    );
    expect(
      await screen.findByText(/does not include the browser hsctl runtime/u),
    ).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
  });

  // Verifies: WEB-TERM-07
  it("shares history with runbook cells and opens at a runbook entry", async () => {
    const user = userEvent.setup();
    renderTerminal();

    await user.click(screen.getByRole("button", { name: "Run step" }));
    act(() => {
      runbook?.appendOutput(["created gateway a"]);
      runbook?.appendOutput(["warning: slow"], "stderr");
      runbook?.finish({ exitCode: 1 });
      runbook?.appendOutput(["ignored after finish"]);
    });
    expect(runbook?.entryId).toBe("entry-1");
    await user.click(screen.getByRole("button", { name: "Open in terminal" }));

    expect(screen.getByTestId("terminal-open").textContent).toBe("true");
    expect(within(log()).getByText("[runbook]")).toBeTruthy();
    expect(within(log()).getByText("created gateway a")).toBeTruthy();
    expect(within(log()).getByText("Exit code 1")).toBeTruthy();
    expect(within(log()).queryByText("ignored after finish")).toBeNull();
    expect(scrollTo).toHaveBeenCalled();
  });

  // Verifies: WEB-TERM-07
  it("clears the history when the session identity changes", async () => {
    const user = userEvent.setup();
    const view = renderTerminal();
    const input = await openTerminal(user);
    await user.type(input, "version{Enter}");
    await within(log()).findByText("version");

    view.rerenderSession({ identity: "grace", status: "signed-in" });

    await waitFor(() => {
      expect(within(log()).queryByText("version")).toBeNull();
    });
    view.rerenderSession({ status: "signed-out" });
    expect(
      screen.getByRole("link", { name: "Sign in to use the hsctl terminal" }),
    ).toBeTruthy();
  });

  // Verifies: WEB-TERM-07.02
  it("never writes commands or output to storage or the URL", async () => {
    const user = userEvent.setup();
    renderTerminal({
      script: {
        respond: () => ({
          exitCode: 0,
          mutated: false,
          stderr: "",
          stdout: "one-time-token-123\n",
        }),
      },
    });
    const input = await openTerminal(user);
    await user.type(input, "create serviceAccount --name ci{Enter}");
    await within(log()).findByText("one-time-token-123");

    const stored = `${storageText(localStorage)}\n${storageText(sessionStorage)}`;
    expect(stored).not.toContain("one-time-token-123");
    expect(stored).not.toContain("serviceAccount");
    expect(window.location.href).not.toContain("serviceAccount");
  });

  // Verifies: WEB-TERM-08.01
  it("scrolls instantly under reduced motion", async () => {
    setMedia({ reducedMotion: true });
    const user = userEvent.setup();
    renderTerminal({
      script: {
        respond: () => ({
          exitCode: 0,
          mutated: false,
          stderr: "",
          stdout: "a\nb\n",
        }),
      },
    });
    const input = await openTerminal(user);
    scrollTo.mockClear();

    await user.type(input, "version{Enter}");
    await within(log()).findByText("b");

    expect(scrollTo).toHaveBeenCalledWith(
      expect.objectContaining({ behavior: "auto" }),
    );
  });

  // Verifies: WEB-TERM-08
  it("animates only lines added after the log mounts and keeps position when scrolled up", async () => {
    const user = userEvent.setup();
    renderTerminal({
      script: {
        respond: () => ({
          exitCode: 0,
          mutated: false,
          stderr: "",
          stdout: Array.from({ length: 10 }, (_v, i) => `l${String(i)}`).join(
            "\n",
          ),
        }),
      },
    });
    await user.click(screen.getByRole("button", { name: "Run step" }));
    const input = await openTerminal(user);
    const existing = within(log()).getByText(/hsctl create gateway/u);
    expect(
      existing.closest("[data-entry-id]")?.getAttribute("style"),
    ).toBeNull();

    await user.type(input, "version{Enter}");
    const last = await within(log()).findByText("l9");
    expect(last.getAttribute("style")).toContain(
      "--terminal-line-delay: 140ms",
    );
    expect(within(log()).getByText("l0").getAttribute("style")).toContain(
      "--terminal-line-delay: 0ms",
    );

    const element = log();
    Object.defineProperty(element, "scrollHeight", { value: 1000 });
    Object.defineProperty(element, "clientHeight", { value: 100 });
    element.scrollTop = 0;
    fireEvent.scroll(element);
    scrollTo.mockClear();
    await user.type(input, "version{Enter}");
    await waitFor(() => {
      expect(within(log()).getAllByText("l9")).toHaveLength(2);
    });
    expect(scrollTo).not.toHaveBeenCalled();
  });

  // Verifies: WEB-TERM-07
  it("says at the top when older lines were dropped", async () => {
    const user = userEvent.setup();
    renderTerminal({
      lineLimit: 3,
      script: {
        respond: () => ({
          exitCode: 0,
          mutated: false,
          stderr: "",
          stdout: "a\nb\nc\nd\n",
        }),
      },
    });
    const input = await openTerminal(user);
    await user.type(input, "version{Enter}");
    expect(
      await screen.findByText(
        "2 older lines were removed. The log keeps the latest 5,000 lines.",
      ),
    ).toBeTruthy();
  });

  // Verifies: WEB-TERM-01
  it("requires the provider", () => {
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    function Orphan() {
      useTerminal();
      return null;
    }
    expect(() => render(<Orphan />)).toThrow(/TerminalProvider/u);
  });
});
