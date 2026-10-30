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
`;

const LINKING_SPEC = `# Console

See [certificate rotation](tls.spec.md#requirement-tls-01----certificates-rotate)
and \`tls.spec.md\`.
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
});

describe("App", () => {
  it("finds a scenario and opens it at its heading", async () => {
    const user = userEvent.setup();
    renderApp();
    const search = await screen.findByRole("textbox", { name: "Search specs" });
    expect(document.activeElement).toBe(search);

    await user.type(search, "expirng");
    const results = await screen.findByRole("tree", { name: "Search results" });
    expect(within(results).getByText("Gateway TLS")).toBeTruthy();
    expect(within(results).getByText("TLS-01")).toBeTruthy();

    await user.click(within(results).getByText("Expiring certificate"));
    const heading = document.getElementById(headingElementId(3));
    expect(heading?.textContent).toBe("Scenario: Expiring certificate");
    expect(heading?.classList.contains(FLASH_CLASS)).toBe(true);
    expect(document.activeElement).toBe(heading);
    expect(decodeURIComponent(window.location.hash)).toBe(
      "#platform/tls.spec.md#scenario: expiring certificate",
    );
  });

  it("highlights the search terms in the open document", async () => {
    const user = userEvent.setup();
    renderApp();
    await user.type(
      await screen.findByRole("textbox", { name: "Search specs" }),
      "certifcate",
    );
    const results = await screen.findByRole("tree", { name: "Search results" });
    await user.click(within(results).getByText("Expiring certificate"));
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

  it("collapses and expands every result", async () => {
    const user = userEvent.setup();
    renderApp();
    await user.type(
      await screen.findByRole("textbox", { name: "Search specs" }),
      "renewed",
    );
    const results = await screen.findByRole("tree", { name: "Search results" });
    expect(within(results).getByText("Expiring certificate")).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "Collapse all" }));
    expect(
      within(screen.getByRole("tree", { name: "Search results" })).queryByText(
        "Expiring certificate",
      ),
    ).toBeNull();

    await user.click(screen.getByRole("button", { name: "Expand all" }));
    expect(
      within(screen.getByRole("tree", { name: "Search results" })).getByText(
        "Expiring certificate",
      ),
    ).toBeTruthy();
  });

  it("labels text outside any spec with its nearest heading", async () => {
    const user = userEvent.setup();
    renderApp();
    await user.type(
      await screen.findByRole("textbox", { name: "Search specs" }),
      "history",
    );
    expect(await screen.findByText("[nospec] Background")).toBeTruthy();
    expect(screen.getByText("history").tagName).toBe("MARK");
  });

  it("pins results and opens them from the pinned panel", async () => {
    const user = userEvent.setup();
    const stored = renderApp();
    await user.type(
      await screen.findByRole("textbox", { name: "Search specs" }),
      "renewed",
    );
    await user.click(
      await screen.findByRole("button", { name: "Pin results" }),
    );
    expect(stored.ids).toEqual([
      "platform/tls.spec.md#scenario: expiring certificate",
    ]);

    const pinned = screen.getByRole("tree", { name: "Pinned specs" });
    await user.click(within(pinned).getByText("Certificates rotate"));
    expect(
      document
        .getElementById(headingElementId(2))
        ?.classList.contains(FLASH_CLASS),
    ).toBe(true);

    await user.click(
      within(pinned).getByRole("button", {
        name: "Unpin Expiring certificate",
      }),
    );
    expect(stored.ids).toEqual([]);
    expect(
      screen.getByText("Pin search results to keep them here."),
    ).toBeTruthy();
  });

  it("opens linked specs inside the viewer", async () => {
    window.history.replaceState(
      null,
      "",
      `#${encodeURIComponent("platform/console.spec.md#console")}`,
    );
    const user = userEvent.setup();
    renderApp();
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
    await user.click(
      await screen.findByRole("button", { name: "Conformance" }),
    );
    const table = screen.getByRole("treegrid", { name: "Spec conformance" });
    expect(within(table).getByText("platform")).toBeTruthy();
    expect(within(table).getByText("Gateway TLS")).toBeTruthy();
    expect(screen.getByText("console.spec.md", { exact: false })).toBeTruthy();

    const fileRow = within(table).getByText("Gateway TLS").closest("tr");
    if (!fileRow) throw new Error("Missing file row");
    await user.click(
      within(fileRow).getByRole("button", { name: /^Expand row/u }),
    );
    await user.click(within(table).getByText("Certificates rotate"));
    expect(
      document
        .getElementById(headingElementId(2))
        ?.classList.contains(FLASH_CLASS),
    ).toBe(true);
  });

  it("lists files that reference the selected ID and opens them at the line", async () => {
    window.history.replaceState(
      null,
      "",
      `#${encodeURIComponent("platform/tls.spec.md#requirement: tls-01 -- certificates rotate")}`,
    );
    const user = userEvent.setup();
    renderApp();
    await user.click(
      await screen.findByRole("button", { name: "Files referencing TLS-01" }),
    );
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
    expect(
      screen.getByRole("heading", { name: "pkg/tls/rotate_test.go" }),
    ).toBeTruthy();
  });

  it("offers no ID view when the selection has no ID", async () => {
    window.history.replaceState(
      null,
      "",
      `#${encodeURIComponent("platform/console.spec.md#console")}`,
    );
    renderApp();
    await screen.findByRole("region", { name: "Spec document" });
    expect(
      screen.queryByRole("button", { name: /^Files referencing/u }),
    ).toBeNull();
  });

  it("has resizable panels and a toggle for the pinned panel", async () => {
    const user = userEvent.setup();
    renderApp();
    await screen.findByRole("textbox", { name: "Search specs" });
    expect(
      screen.getByRole("separator", { name: "Resize the pinned panel" }),
    ).toBeTruthy();
    expect(
      screen.getByRole("separator", { name: "Resize the list panel" }),
    ).toBeTruthy();

    const toggle = screen.getByRole("button", { name: "Pinned" });
    expect(toggle.getAttribute("aria-expanded")).toBe("true");
    await user.click(toggle);
    expect(toggle.getAttribute("aria-expanded")).toBe("false");
  });

  it("shows which test verifies a scenario and opens it", async () => {
    const user = userEvent.setup();
    renderApp();
    await user.click(
      await screen.findByRole("button", { name: "Conformance" }),
    );
    const table = screen.getByRole("treegrid", { name: "Spec conformance" });
    const expand = async (text: string) => {
      const row = (await within(table).findByText(text)).closest("tr");
      if (!row) throw new Error(`Missing row ${text}`);
      await user.click(
        within(row).getByRole("button", { name: /^Expand row/u }),
      );
    };
    await expand("Traced");
    await expand("Quokka handling");

    await user.click(await within(table).findByText("TestFeedsQuokka"));
    const source = await screen.findByRole("region", { name: "Source file" });
    await waitFor(() => {
      expect(source.querySelectorAll(".line")[2]?.classList).toContain(
        FLASH_CLASS,
      );
    });
    expect(screen.getByText("Tested scenarios")).toBeTruthy();
  });

  it("restores saved pins and the selection from the URL", async () => {
    window.history.replaceState(
      null,
      "",
      `#${encodeURIComponent("platform/tls.spec.md#background")}`,
    );
    renderApp(["platform/tls.spec.md#background"]);
    const pinned = await screen.findByRole("tree", { name: "Pinned specs" });
    expect(within(pinned).getByText("[nospec] Background")).toBeTruthy();
    await waitFor(() => {
      expect(
        document
          .getElementById(headingElementId(4))
          ?.classList.contains(FLASH_CLASS),
      ).toBe(true);
    });
  });

  it("switches to another branch and keeps it in the URL", async () => {
    const user = userEvent.setup();
    renderApp();
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

  it("opens the branch named in the URL", async () => {
    window.history.replaceState(null, "", "/?ref=feature%2Fx");
    renderApp();
    expect(await screen.findByText(/^1 file/u)).toBeTruthy();
  });
});
