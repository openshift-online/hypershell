import { afterEach, describe, expect, it } from "vitest";

import { createBrowserHistory, createMemoryHistory } from "./history-port";

afterEach(() => {
  window.history.replaceState(null, "", "/");
});

describe("createBrowserHistory base-path handling (FLT-NAV-19)", () => {
  it("round-trips a reverse-proxy base path across read and push", () => {
    // Simulate the document being served under a base path, deep-linked to a tab.
    window.history.replaceState(null, "", "/glass/e2e?run=t%2Fn%2Fr");
    const hist = createBrowserHistory();

    // read() strips the base path, handing the codec a root-relative location.
    expect(hist.read()).toEqual({ path: "/e2e", search: "?run=t%2Fn%2Fr" });

    // push re-applies the base path, so reload and copy-link keep working.
    hist.push({ path: "/sdlc", search: "?q=x" });
    expect(window.location.pathname).toBe("/glass/sdlc");
    expect(window.location.search).toBe("?q=x");
    expect(hist.read()).toEqual({ path: "/sdlc", search: "?q=x" });

    // The map tab serializes to the root; under the base path that is the base root.
    hist.replace({ path: "/", search: "" });
    expect(window.location.pathname).toBe("/glass/");
  });

  it("uses no prefix at the document root", () => {
    window.history.replaceState(null, "", "/e2e");
    const hist = createBrowserHistory();
    expect(hist.read().path).toBe("/e2e");
    hist.push({ path: "/sdlc", search: "" });
    expect(window.location.pathname).toBe("/sdlc");
  });
});

describe("createMemoryHistory Back/Forward stack", () => {
  it("navigates a real history stack", () => {
    const hist = createMemoryHistory({ path: "/", search: "" });
    let notified = 0;
    hist.subscribe(() => {
      notified += 1;
    });
    hist.push({ path: "/sdlc", search: "" });
    hist.push({ path: "/e2e", search: "" });
    expect(hist.read().path).toBe("/e2e");
    hist.back();
    expect(hist.read().path).toBe("/sdlc");
    hist.back();
    expect(hist.read().path).toBe("/");
    hist.forward();
    expect(hist.read().path).toBe("/sdlc");
    expect(notified).toBe(3);
    // A push after going back truncates the forward stack.
    hist.push({ path: "/e2e", search: "" });
    hist.forward();
    expect(hist.read().path).toBe("/e2e");
  });
});
