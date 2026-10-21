import { act, fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { RouteLocation } from "../../domain/navigation/route";
import type { HistoryPort } from "./history-port";
import { RouterProvider, useNavLink, useRouter } from "./router";

/** An in-memory HistoryPort that records pushes and replaces and can emit popstate. */
function fakeHistory(
  initial: RouteLocation,
  basePath = "",
): {
  port: HistoryPort;
  pushes: string[];
  replaces: string[];
  set: (loc: RouteLocation) => void;
} {
  let current = initial;
  let listener: (() => void) | null = null;
  const pushes: string[] = [];
  const replaces: string[] = [];
  const toHref = (l: RouteLocation) => `${l.path}${l.search}`;
  return {
    pushes,
    replaces,
    set: (loc) => {
      current = loc;
      listener?.();
    },
    port: {
      read: () => current,
      // Link hrefs carry the base path, matching the production adapter (FLT-NAV-19).
      href: (loc) => basePath + toHref(loc),
      push: (loc) => {
        current = loc;
        pushes.push(toHref(loc));
      },
      replace: (loc) => {
        current = loc;
        replaces.push(toHref(loc));
      },
      subscribe: (cb) => {
        listener = cb;
        return () => {
          listener = null;
        };
      },
    },
  };
}

const LINK_LABEL = "e2e link";

function Probe(): React.ReactElement {
  const { route, navigate, hrefFor } = useRouter();
  const link = useNavLink({ type: "tab", tab: "e2e" });
  return (
    <div>
      <output data-testid="tab">{route.tab}</output>
      <output data-testid="run">{route.e2e.run ?? ""}</output>
      <output data-testid="runq">{route.e2e.query}</output>
      <button
        data-testid="go-sdlc"
        onClick={() => {
          navigate({ type: "tab", tab: "sdlc" });
        }}
      />
      <button
        data-testid="select-run"
        onClick={() => {
          navigate({ type: "e2eSelectRun", run: "t/n/r1" });
        }}
      />
      <button
        data-testid="type"
        onClick={() => {
          navigate({ type: "e2eQuery", value: "abc" });
        }}
      />
      <span data-testid="href">
        {hrefFor({ type: "e2eSelectRun", run: "t/n/r2" })}
      </span>
      {/* Content via a variable, not a JSX string literal, so the anchor has an
          accessible name without tripping the i18n lint. */}
      <a data-testid="link" {...link}>
        {LINK_LABEL}
      </a>
    </div>
  );
}

function renderWithHistory(initial: RouteLocation, basePath = "") {
  const h = fakeHistory(initial, basePath);
  render(
    <RouterProvider history={h.port}>
      <Probe />
    </RouterProvider>,
  );
  return h;
}

describe("RouterProvider", () => {
  it("hydrates route state from the initial URL before anything else (FLT-NAV-05)", () => {
    renderWithHistory({ path: "/e2e", search: "?run=t%2Fn%2Fr0" });
    expect(screen.getByTestId("tab").textContent).toBe("e2e");
    expect(screen.getByTestId("run").textContent).toBe("t/n/r0");
  });

  it("pushes for meaningful navigation (FLT-NAV-06)", () => {
    const h = renderWithHistory({ path: "/", search: "" });
    fireEvent.click(screen.getByTestId("go-sdlc"));
    expect(screen.getByTestId("tab").textContent).toBe("sdlc");
    expect(h.pushes).toEqual(["/sdlc"]);
    expect(h.replaces).toEqual([]);
  });

  // Verifies: FLT-NAV-06.01
  it("replaces (coalesces) for text input (FLT-NAV-06)", () => {
    const h = renderWithHistory({ path: "/e2e", search: "" });
    fireEvent.click(screen.getByTestId("type"));
    expect(screen.getByTestId("runq").textContent).toBe("abc");
    expect(h.pushes).toEqual([]);
    expect(h.replaces).toEqual(["/e2e?runq=abc"]);
  });

  // Verifies: FLT-NAV-07.01
  it("reconciles from the URL on popstate without echoing a new entry (FLT-NAV-07)", () => {
    const h = renderWithHistory({ path: "/", search: "" });
    act(() => {
      h.set({ path: "/e2e", search: "?run=t%2Fn%2Fr9" });
    });
    expect(screen.getByTestId("tab").textContent).toBe("e2e");
    expect(screen.getByTestId("run").textContent).toBe("t/n/r9");
    // Reacting to Back/Forward must not create history entries.
    expect(h.pushes).toEqual([]);
    expect(h.replaces).toEqual([]);
  });

  it("does not create a duplicate entry for a no-op navigation", () => {
    const h = renderWithHistory({ path: "/sdlc", search: "" });
    fireEvent.click(screen.getByTestId("go-sdlc"));
    expect(h.pushes).toEqual([]);
  });

  it("builds real hrefs for links and navigates on a plain click (FLT-NAV-08)", () => {
    const h = renderWithHistory({ path: "/", search: "" });
    // Selecting a run keeps the current tab (map here); the path stays "/".
    expect(screen.getByTestId("href").textContent).toBe("/?run=t%2Fn%2Fr2");
    const link = screen.getByTestId("link");
    expect(link.getAttribute("href")).toBe("/e2e");
    fireEvent.click(link, { button: 0 });
    expect(screen.getByTestId("tab").textContent).toBe("e2e");
    expect(h.pushes).toEqual(["/e2e"]);
  });

  it("includes the reverse-proxy base path in link hrefs (FLT-NAV-08/19)", () => {
    // Under a base-path deployment, copy-link/open-in-new-tab/Cmd-click use the shown
    // `<a href>`, so it must carry the base path, not the codec's root-relative path.
    renderWithHistory({ path: "/", search: "" }, "/glass");
    expect(screen.getByTestId("link").getAttribute("href")).toBe("/glass/e2e");
    expect(screen.getByTestId("href").textContent).toBe(
      "/glass/?run=t%2Fn%2Fr2",
    );
  });

  it("lets the browser handle modified clicks (open-in-new-tab)", () => {
    const h = renderWithHistory({ path: "/", search: "" });
    fireEvent.click(screen.getByTestId("link"), { button: 0, metaKey: true });
    expect(screen.getByTestId("tab").textContent).toBe("map");
    expect(h.pushes).toEqual([]);
  });
});
