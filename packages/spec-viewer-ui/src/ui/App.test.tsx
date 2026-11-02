import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { IntlProvider } from "react-intl";

import { markedMarkdown } from "../adapters/marked-markdown";
import type { PinStorePort, SpecViewerPorts } from "../application/ports";
import { App } from "./App";
import { PortsProvider } from "./ports-context";
import { MATCH_CLASS } from "./highlight-html";
import { FLASH_CLASS, headingElementId } from "./SpecDocument";

const TRACED_SPEC = `# Traced

### Requirement: TRC-FED-01 -- Quokka handling

The quokka SHALL be handled.

#### Scenario: TRC-FED-01.01 -- Quokka is fed

- WHEN the quokka is hungry
- THEN it is fed

### Requirement: TRC-FED-02 -- Quokka naps

The quokka SHALL nap.

#### Scenario: TRC-FED-02.01 -- Nap without outcome

- WHEN the quokka is tired

### Requirement: TRC-FED-03 -- Quokka is reviewed

**Verification:** Inspection -- a keeper checks by hand.

The quokka SHALL be calm.

#### Scenario: TRC-FED-03.01 -- Keeper looks

- WHEN a keeper looks
- THEN the quokka is calm
`;

const LINKING_SPEC = `# Console

See [certificate rotation](tls.spec.md#requirement-tls-01----certificates-rotate)
and \`tls.spec.md\`.

Builds on TLS-01 and \`TRC-FED-01.01\`, over HTTP-2.
`;

const SPEC = `# Gateway TLS

## Requirements

### Requirement: TLS-01 -- Certificates rotate

Certificates SHALL rotate before expiry.

#### Scenario: Expiring certificate

- WHEN a certificate expires within a day
- THEN it SHALL be renewed

## Background

Rotation history lives elsewhere.
`;

const BRANCH_SPEC = `# Branch only

### Requirement: Only on the branch

The feature SHALL exist.
`;

function renderApp(saved: string[] = []) {
  const stored = { ids: saved };
  const pins: PinStorePort = {
    load: () => stored.ids,
    save: (ids) => {
      stored.ids = ids;
    },
  };
  const ports: SpecViewerPorts = {
    specs: {
      loadSpecFiles: (ref) =>
        Promise.resolve(
          ref === "feature/x"
            ? [{ path: "platform/branch.spec.md", content: BRANCH_SPEC }]
            : [
                { path: "platform/tls.spec.md", content: SPEC },
                { path: "platform/console.spec.md", content: LINKING_SPEC },
                { path: "platform/traced.spec.md", content: TRACED_SPEC },
              ],
        ),
      subscribe: () => () => undefined,
    },
    markdown: markedMarkdown,
    pins,
    repository: {
      listRefs: () =>
        Promise.resolve({ default: "main", refs: ["feature/x", "main"] }),
      findReferences: (_ref, id) =>
        Promise.resolve(
          id === "TLS-01"
            ? [
                {
                  path: "pkg/tls/rotate_test.go",
                  isTest: true,
                  matches: [{ line: 3, text: "// Covers TLS-01." }],
                },
              ]
            : [],
        ),
      loadMarkers: () =>
        Promise.resolve([
          {
            path: "pkg/tls/rotate_test.go",
            line: 3,
            text: "// Verifies: TRC-FED-01.01",
            following: ["func TestFeedsQuokka(t *testing.T) {"],
          },
        ]),
      readFile: () =>
        Promise.resolve(
          "package tls\n\n// Covers TLS-01.\nfunc TestRotate() {}\n",
        ),
    },
    highlighter: {
      highlight: (code) =>
        Promise.resolve(
          `<pre class="shiki"><code>${code
            .split("\n")
            .map((line) => `<span class="line">${line}</span>`)
            .join("\n")}</code></pre>`,
        ),
    },
  };
  render(
    <IntlProvider defaultLocale="en" locale="en">
      <QueryClientProvider client={new QueryClient()}>
        <PortsProvider ports={ports}>
          <App />
        </PortsProvider>
      </QueryClientProvider>
    </IntlProvider>,
  );
  return stored;
}

beforeEach(() => {
  window.history.replaceState(null, "", "/");
  // The viewer remembers the view, panel widths, and pins in localStorage.
  // Where storage works (CI), one test's choices would leak into the next.
  // Without it (Node with no --localstorage-file), there is nothing to clear.
  try {
    (window.localStorage as Storage | undefined)?.clear();
  } catch {
    // Blocked storage: nothing was kept.
  }
});

/** The conformance table, the main view. */
const table = () => screen.getByRole("treegrid", { name: "Spec conformance" });

/** Waits for the specs to load and the table to show. */
async function ready() {
  await screen.findByRole("treegrid", { name: "Spec conformance" });
}

async function search(user: ReturnType<typeof userEvent.setup>, text: string) {
  await user.type(
    await screen.findByRole("textbox", { name: "Search specs" }),
    text,
  );
}

/** Expands the table row that holds `text`. */
async function expand(user: ReturnType<typeof userEvent.setup>, text: string) {
  const row = (await within(table()).findByText(text)).closest("tr");
  if (!row) throw new Error(`Missing row ${text}`);
  await user.click(within(row).getByRole("button", { name: /^Expand row/u }));
}

describe("App", () => {
  // Verifies: DEV-SPV-09.01
  it("filters the table by a search and opens a scenario", async () => {
    const user = userEvent.setup();
    renderApp();
    await ready();
    const input = await screen.findByRole("textbox", { name: "Search specs" });
    expect(document.activeElement).toBe(input);
    expect(within(table()).getByText("Traced")).toBeTruthy();

    await search(user, "expirng");
    // The matching scenario shows, with the rows above it expanded.
    expect(
      await within(table()).findByText("Expiring certificate"),
    ).toBeTruthy();
    expect(within(table()).getByText("Gateway TLS")).toBeTruthy();
    expect(within(table()).queryByText("Traced")).toBeNull();
    expect(screen.getByText("1 match in 1 file")).toBeTruthy();

    await user.click(within(table()).getByText("Expiring certificate"));
    const heading = document.getElementById(headingElementId(3));
    expect(heading?.textContent).toBe("Scenario: Expiring certificate");
    expect(heading?.classList.contains(FLASH_CLASS)).toBe(true);
    expect(decodeURIComponent(window.location.hash)).toBe(
      "#platform/tls.spec.md#scenario: expiring certificate",
    );
  });

  it("highlights the search terms in the open document", async () => {
    const user = userEvent.setup();
    renderApp();
    await ready();
    await search(user, "certifcate");
    await user.click(await within(table()).findByText("Expiring certificate"));
    const document = screen.getByRole("region", { name: "Spec document" });
    const marks = [...document.querySelectorAll(`mark.${MATCH_CLASS}`)].map(
      (mark) => mark.textContent,
    );
    expect(marks).toEqual([
      "Certificates",
      "Certificates",
      "certificate",
      "certificate",
    ]);
  });

  // Verifies: DEV-SPV-09.02
  it("shows the matching text when hovering a title", async () => {
    const user = userEvent.setup();
    renderApp();
    await ready();
    await search(user, "renewed");
    await user.hover(await within(table()).findByText("Expiring certificate"));
    const tooltip = await screen.findByRole("tooltip");
    expect(within(tooltip).getByText("renewed").tagName).toBe("MARK");
  });

  it("shows text outside requirements under its file", async () => {
    const user = userEvent.setup();
    renderApp();
    await ready();
    await search(user, "history");
    await user.hover(await within(table()).findByText("Gateway TLS"));
    const tooltip = await screen.findByRole("tooltip");
    expect(within(tooltip).getByText("platform/tls.spec.md")).toBeTruthy();
    expect(within(tooltip).getByText("[nospec] Background")).toBeTruthy();
    expect(within(tooltip).getByText("history").tagName).toBe("MARK");
    expect(within(table()).queryByText("Certificates rotate")).toBeNull();
  });

  // Verifies: DEV-SPV-10.01
  it("stars items, filters to them, and unstars them", async () => {
    const user = userEvent.setup();
    const stored = renderApp();
    await ready();
    await expand(user, "Gateway TLS");
    await user.click(
      within(table()).getByRole("button", { name: "Star Certificates rotate" }),
    );
    expect(stored.ids).toEqual([
      "platform/tls.spec.md#requirement: tls-01 -- certificates rotate",
    ]);
    expect(
      within(table())
        .getByRole("button", { name: "Unstar Certificates rotate" })
        .getAttribute("aria-pressed"),
    ).toBe("true");

    await user.click(
      screen.getByRole("button", { name: "Show only starred items" }),
    );
    // A starred requirement brings its scenarios.
    expect(
      await within(table()).findByText("Expiring certificate"),
    ).toBeTruthy();
    expect(within(table()).queryByText("Traced")).toBeNull();

    await user.click(
      within(table()).getByRole("button", {
        name: "Unstar Certificates rotate",
      }),
    );
    expect(stored.ids).toEqual([]);
    await waitFor(() => {
      expect(within(table()).queryByText("Certificates rotate")).toBeNull();
    });
  });

  it("opens linked specs inside the viewer", async () => {
    window.history.replaceState(
      null,
      "",
      `#${encodeURIComponent("platform/console.spec.md#console")}`,
    );
    const user = userEvent.setup();
    renderApp();
    await ready();
    const document = await screen.findByRole("region", {
      name: "Spec document",
    });
    const codeLink = within(document).getByRole("link", {
      name: "tls.spec.md",
    });
    expect(decodeURIComponent(codeLink.getAttribute("href") ?? "")).toBe(
      "#platform/tls.spec.md#gateway tls",
    );

    await user.click(
      within(document).getByRole("link", { name: "certificate rotation" }),
    );
    await waitFor(() => {
      expect(
        screen.getByRole("heading", { level: 2, name: "platform/tls.spec.md" }),
      ).toBeTruthy();
    });
    expect(
      document.ownerDocument
        .getElementById(headingElementId(2))
        ?.classList.contains(FLASH_CLASS),
    ).toBe(true);
  });

  it("shows spec conformance and opens a requirement from it", async () => {
    const user = userEvent.setup();
    renderApp();
    await ready();
    expect(await within(table()).findByText("platform")).toBeTruthy();
    expect(within(table()).getByText("Gateway TLS")).toBeTruthy();
    // A file's path is not in the row; it shows on hover.
    expect(within(table()).queryByText("platform/tls.spec.md")).toBeNull();
    await user.hover(within(table()).getByText("Gateway TLS"));
    const tooltip = await screen.findByRole("tooltip");
    expect(within(tooltip).getByText("platform/tls.spec.md")).toBeTruthy();
    await user.unhover(within(table()).getByText("Gateway TLS"));

    await expand(user, "Gateway TLS");
    await user.click(within(table()).getByText("Certificates rotate"));
    expect(
      document
        .getElementById(headingElementId(2))
        ?.classList.contains(FLASH_CLASS),
    ).toBe(true);
  });

  // Verifies: DEV-SPV-11.01, DEV-SPV-11.02
  it("labels each requirement's problems, or how it is verified", async () => {
    const user = userEvent.setup();
    renderApp();
    await ready();
    await expand(user, "Traced");
    const row = (title: string) => {
      const found = within(table()).getByText(title).closest("tr");
      if (!found) throw new Error(`Missing row ${title}`);
      return within(found);
    };
    // A scenario with WHEN and no THEN fails its requirement.
    expect(row("Quokka naps").getByText("malformed scenarios")).toBeTruthy();
    await expand(user, "Quokka naps");
    expect(row("Nap without outcome").getByText("missing THEN")).toBeTruthy();
    // An inspection-verified requirement needs no test.
    const reviewed = row("Quokka is reviewed");
    expect(reviewed.getByText("Verified by Inspection")).toBeTruthy();
    expect(reviewed.queryByText("untested scenarios")).toBeNull();
    expect(reviewed.getByText("ok")).toBeTruthy();
  });

  // Verifies: DEV-SPV-13.01, DEV-SPV-13.02
  it("finds an ID's references from its label and goes back", async () => {
    const user = userEvent.setup();
    renderApp();
    await ready();
    // The search expands Gateway TLS; the requirement is expanded by hand.
    await search(user, "rotate");
    await expand(user, "Certificates rotate");
    expect(
      await within(table()).findByText("Expiring certificate"),
    ).toBeTruthy();
    await user.click(within(table()).getByRole("button", { name: "TLS-01" }));

    const tests = await screen.findByRole("tree", { name: "1 test file" });
    await user.click(within(tests).getByText("// Covers TLS-01."));
    const source = await screen.findByRole("region", { name: "Source file" });
    await waitFor(() => {
      expect(source.querySelectorAll(".line")).toHaveLength(5);
    });
    const line = source.querySelectorAll(".line")[2];
    expect(line?.classList.contains(FLASH_CLASS)).toBe(true);
    expect(line?.querySelector(`mark.${MATCH_CLASS}`)?.textContent).toBe(
      "TLS-01",
    );

    await user.click(screen.getByRole("button", { name: "Back" }));
    // The same search and the same expanded rows.
    expect(
      screen.getByRole<HTMLInputElement>("textbox", { name: "Search specs" })
        .value,
    ).toBe("rotate");
    expect(within(table()).getByText("Expiring certificate")).toBeTruthy();
    expect(within(table()).queryByText("Traced")).toBeNull();
  });

  it("has a resizable list panel", async () => {
    renderApp();
    await ready();
    expect(
      screen.getByRole("separator", { name: "Resize the list panel" }),
    ).toBeTruthy();
  });

  // Verifies: DEV-SPV-14.01
  it("finds an ID's references from a label in the document", async () => {
    window.history.replaceState(
      null,
      "",
      `#${encodeURIComponent("platform/console.spec.md#console")}`,
    );
    const user = userEvent.setup();
    renderApp();
    await ready();
    const document = await screen.findByRole("region", {
      name: "Spec document",
    });
    // Known IDs become labels, in text and in inline code; others do not.
    expect(
      within(document).getByRole("button", { name: "TRC-FED-01.01" }),
    ).toBeTruthy();
    expect(
      within(document).queryByRole("button", { name: "HTTP-2" }),
    ).toBeNull();

    await user.hover(within(document).getByRole("button", { name: "TLS-01" }));
    expect((await screen.findByRole("tooltip")).textContent).toBe(
      "Find references",
    );
    await user.click(within(document).getByRole("button", { name: "TLS-01" }));
    expect(
      await screen.findByRole("tree", { name: "1 test file" }),
    ).toBeTruthy();
  });

  it("shows which test verifies a scenario and opens it", async () => {
    const user = userEvent.setup();
    renderApp();
    await ready();
    await expand(user, "Traced");
    await expand(user, "Quokka handling");

    await user.click(await within(table()).findByText("TestFeedsQuokka"));
    const source = await screen.findByRole("region", { name: "Source file" });
    await waitFor(() => {
      expect(source.querySelectorAll(".line")[2]?.classList).toContain(
        FLASH_CLASS,
      );
    });
    expect(screen.getByText("Tested scenarios")).toBeTruthy();
  });

  // Verifies: DEV-SPV-14.02
  it("restores saved stars and the selection from the URL", async () => {
    window.history.replaceState(
      null,
      "",
      `#${encodeURIComponent("platform/tls.spec.md#background")}`,
    );
    const user = userEvent.setup();
    renderApp([
      "platform/tls.spec.md#requirement: tls-01 -- certificates rotate",
    ]);
    await ready();
    await waitFor(() => {
      expect(
        document
          .getElementById(headingElementId(4))
          ?.classList.contains(FLASH_CLASS),
      ).toBe(true);
    });
    await user.click(
      screen.getByRole("button", { name: "Show only starred items" }),
    );
    expect(
      await within(table()).findByRole("button", {
        name: "Unstar Certificates rotate",
      }),
    ).toBeTruthy();
  });

  it("switches to another branch and keeps it in the URL", async () => {
    const user = userEvent.setup();
    renderApp();
    await ready();
    expect(await screen.findByText(/^3 files/u)).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "Branch" }));
    const options = await screen.findAllByRole("option");
    expect(options.map((option) => option.textContent)).toEqual([
      "mainDefault branch",
      "feature/x",
    ]);
    await user.click(screen.getByRole("option", { name: "feature/x" }));

    expect(await screen.findByText(/^1 file/u)).toBeTruthy();
    expect(window.location.search).toBe("?ref=feature%2Fx");

    await user.click(screen.getByRole("button", { name: "Branch" }));
    await user.click(await screen.findByRole("option", { name: /^main/u }));
    expect(await screen.findByText(/^3 files/u)).toBeTruthy();
    expect(window.location.search).toBe("");
  });

  // Verifies: DEV-SPV-06.01
  it("opens the branch named in the URL", async () => {
    window.history.replaceState(null, "", "/?ref=feature%2Fx");
    renderApp();
    await ready();
    expect(await screen.findByText(/^1 file/u)).toBeTruthy();
  });

  it("hides, shows, maximizes, and restores the document", async () => {
    const user = userEvent.setup();
    renderApp();
    await ready();
    await screen.findByRole("textbox", { name: "Search specs" });
    expect(screen.getByText("No spec selected")).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "Hide document" }));
    expect(screen.queryByText("No spec selected")).toBeNull();
    expect(
      screen.queryByRole("separator", { name: "Resize the list panel" }),
    ).toBeNull();

    // Opening an item while the document is hidden shows it again.
    await search(user, "expirng");
    await user.click(await within(table()).findByText("Expiring certificate"));
    expect(
      await screen.findByRole("button", { name: "Hide document" }),
    ).toBeTruthy();
    expect(
      screen.getByRole("separator", { name: "Resize the list panel" }),
    ).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "Maximize document" }));
    const restore = screen.getByRole("button", { name: "Restore panels" });
    expect(restore.getAttribute("aria-pressed")).toBe("true");
    await user.click(restore);
    expect(
      screen.getByRole("button", { name: "Maximize document" }),
    ).toBeTruthy();
  });

  // Verifies: DEV-SPV-15.01
  it("follows the system theme until one is chosen", async () => {
    // jsdom has no matchMedia; stand in a system that prefers dark mode.
    const original = Object.getOwnPropertyDescriptor(window, "matchMedia");
    Object.defineProperty(window, "matchMedia", {
      configurable: true,
      value: (query: string) => ({
        matches: query === "(prefers-color-scheme: dark)",
        media: query,
        addEventListener: () => undefined,
        removeEventListener: () => undefined,
      }),
    });
    try {
      renderApp();
      await ready();
      expect(
        document.documentElement.classList.contains("pf-v6-theme-dark"),
      ).toBe(true);
    } finally {
      if (original) Object.defineProperty(window, "matchMedia", original);
      else Reflect.deleteProperty(window, "matchMedia");
      document.documentElement.classList.remove("pf-v6-theme-dark");
    }
  });

  it("switches between the light and dark themes", async () => {
    const user = userEvent.setup();
    renderApp();
    await ready();
    await screen.findByRole("textbox", { name: "Search specs" });
    const root = document.documentElement;
    expect(root.classList.contains("pf-v6-theme-dark")).toBe(false);

    await user.click(
      screen.getByRole("button", { name: "Switch to dark theme" }),
    );
    expect(root.classList.contains("pf-v6-theme-dark")).toBe(true);

    await user.click(
      screen.getByRole("button", { name: "Switch to light theme" }),
    );
    expect(root.classList.contains("pf-v6-theme-dark")).toBe(false);
  });
});
