import { describe, expect, it } from "vitest";

import {
  DEFAULT_ROUTE,
  GATE_LIVE_ATTEMPT,
  ROUTE_LIMITS,
  normalizeRoute,
  parseHref,
  parseRoute,
  routeToHref,
  serializeRoute,
  type RouteState,
} from "./route";

/** Parse a full href and re-serialize it to a canonical href. */
function canonical(href: string): string {
  return routeToHref(parseHref(href));
}

describe("parseRoute / serializeRoute", () => {
  it("defaults to the Fleet Map with nothing selected", () => {
    expect(parseRoute({ path: "/", search: "" })).toEqual(DEFAULT_ROUTE);
    expect(routeToHref(DEFAULT_ROUTE)).toBe("/");
  });

  it("maps the path to the tab and omits the default tab from the path", () => {
    expect(parseHref("/sdlc").tab).toBe("sdlc");
    expect(parseHref("/e2e").tab).toBe("e2e");
    expect(parseHref("/map").tab).toBe("map");
    // Canonical path for the default tab is the root.
    expect(routeToHref({ ...DEFAULT_ROUTE, tab: "map" })).toBe("/");
    expect(routeToHref({ ...DEFAULT_ROUTE, tab: "e2e" })).toBe("/e2e");
  });

  it("accepts a base-path prefix on the tab segment (GLASS-NAV-19)", () => {
    expect(parseHref("/glass/e2e").tab).toBe("e2e");
    expect(parseHref("/some/base/sdlc").tab).toBe("sdlc");
  });

  it("round-trips a rich map gate view", () => {
    const href =
      "/?gate=staging-%3Ecanary&gattempt=t%2Fns%2Fr1&gtab=logs&gstep=build&glog=1&glogq=error";
    expect(canonical(href)).toBe(href);
    const state = parseHref(href);
    expect(state.map.gate).toBe("staging->canary");
    expect(state.map.attempt).toBe("t/ns/r1");
    expect(state.map.exec.tab).toBe("logs");
    expect(state.map.exec.step).toBe("build");
    expect(state.map.exec.logExpanded).toBe(true);
    expect(state.map.exec.logQuery).toBe("error");
  });

  it("round-trips an e2e run view with sorts and log scope", () => {
    const href =
      "/e2e?runq=flake&runsort=4-asc&run=tenant%2Fns%2Frun-1&rtab=logs&rstep=step-2&rssort=1-asc";
    expect(canonical(href)).toBe(href);
    const state = parseHref(href);
    expect(state.e2e.query).toBe("flake");
    expect(state.e2e.runSort).toEqual({ col: 4, dir: "asc" });
    expect(state.e2e.run).toBe("tenant/ns/run-1");
    expect(state.e2e.drawer).toBe(true);
    expect(state.e2e.exec.step).toBe("step-2");
    expect(state.e2e.exec.stepSort).toEqual({ col: 1, dir: "asc" });
  });

  it("distinguishes the explicit live gate mode from a pinned attempt (GLASS-NAV-10)", () => {
    const live = "/?gate=g1&gattempt=live";
    expect(canonical(live)).toBe(live);
    expect(parseHref(live).map.attempt).toBe(GATE_LIVE_ATTEMPT);
    const pinned = parseHref("/?gate=g1&gattempt=t%2Fn%2Fr1");
    expect(pinned.map.attempt).toBe("t/n/r1");
    expect(pinned.map.attempt).not.toBe(GATE_LIVE_ATTEMPT);
  });

  it("encodes the selected-but-closed drawer as rund=0 (GLASS-NAV-12)", () => {
    const open = parseHref("/e2e?run=t%2Fn%2Fr");
    expect(open.e2e.drawer).toBe(true);
    const closed: RouteState = {
      ...open,
      e2e: { ...open.e2e, drawer: false },
    };
    expect(routeToHref(closed)).toBe("/e2e?run=t%2Fn%2Fr&rund=0");
    // And it round-trips back to selected-but-closed.
    expect(parseHref("/e2e?run=t%2Fn%2Fr&rund=0").e2e.drawer).toBe(false);
  });
});

describe("stable identity and SDLC state", () => {
  it("addresses the shared bundle, card, and shipped by identity (GLASS-NAV-02)", () => {
    const href =
      "/sdlc?bundle=sha256%3Aabc&scope=a&scope=b&q=511&card=511&shipped=490&shippedVer=v2026&escalations=1";
    const state = parseHref(href);
    expect(state.bundle).toBe("sha256:abc");
    expect(state.sdlc.card).toBe(511);
    expect(state.sdlc.shipped).toBe(490);
    expect(state.sdlc.shippedVersion).toBe("v2026");
    expect(state.sdlc.escalations).toBe(true);
  });

  it("orders scopes deterministically and deduplicates (GLASS-NAV-13)", () => {
    const a = canonical("/sdlc?scope=zed&scope=amber&scope=amber");
    const b = canonical("/sdlc?scope=amber&scope=zed");
    expect(a).toBe(b);
    expect(parseHref(a).sdlc.scopes).toEqual(["amber", "zed"]);
  });

  it("drops shippedVersion when no shipped issue is selected", () => {
    const state = parseHref("/sdlc?shippedVer=v2026");
    expect(state.sdlc.shippedVersion).toBeNull();
    expect(routeToHref(state)).toBe("/sdlc");
  });
});

describe("canonicalization and equivalence (GLASS-NAV-02)", () => {
  it("omits default-valued state", () => {
    expect(canonical("/e2e?runsort=2-desc")).not.toContain("runsort");
    // Explicitly default exec values are dropped.
    expect(canonical("/e2e?run=t%2Fn%2Fr&rtab=progress")).toBe(
      "/e2e?run=t%2Fn%2Fr",
    );
    expect(canonical("/?node=n1&nodeTab=details")).toBe("/?node=n1");
  });

  it("produces equal URLs for equivalent views regardless of input order", () => {
    const a = canonical("/e2e?run=t%2Fn%2Fr&runq=x");
    const b = canonical("/e2e?runq=x&run=t%2Fn%2Fr");
    expect(a).toBe(b);
  });

  it("is idempotent under normalize", () => {
    const state = parseHref("/e2e?run=t%2Fn%2Fr&rtab=logs&rstep=s1");
    expect(normalizeRoute(normalizeRoute(state))).toEqual(
      normalizeRoute(state),
    );
  });
});

describe("mutually exclusive and incompatible selections (GLASS-NAV-14)", () => {
  it("prefers node over gate when both are present", () => {
    const state = parseHref("/?node=n1&gate=g1");
    expect(state.map.node).toBe("n1");
    expect(state.map.gate).toBeNull();
    expect(routeToHref(state)).toBe("/?node=n1");
  });

  it("drops gate execution detail when no gate is selected", () => {
    const state = parseHref("/?node=n1&gtab=logs&gstep=build");
    expect(state.map.exec.tab).toBe("progress");
    expect(state.map.exec.step).toBeNull();
  });

  it("drops run execution detail when the drawer is closed", () => {
    const state = parseHref("/e2e?run=t%2Fn%2Fr&rund=0&rtab=logs&rstep=s1");
    expect(state.e2e.exec.tab).toBe("progress");
    expect(state.e2e.exec.step).toBeNull();
    expect(routeToHref(state)).toBe("/e2e?run=t%2Fn%2Fr&rund=0");
  });
});

describe("malformed and unknown parameters degrade safely (GLASS-NAV-14)", () => {
  it("rejects an unknown tab segment and falls back to the map", () => {
    expect(parseHref("/bogus").tab).toBe("map");
  });

  it("rejects an unknown enum value", () => {
    expect(parseHref("/?node=n1&nodeTab=wat").map.nodeTab).toBe("details");
    expect(parseHref("/e2e?run=t%2Fn%2Fr&rtab=wat").e2e.exec.tab).toBe(
      "progress",
    );
  });

  it("rejects out-of-range and malformed sorts", () => {
    expect(parseHref("/e2e?runsort=99-asc").e2e.runSort).toBeNull();
    expect(parseHref("/e2e?runsort=2-sideways").e2e.runSort).toBeNull();
    expect(parseHref("/e2e?runsort=garbage").e2e.runSort).toBeNull();
  });

  it("rejects non-positive and junk issue numbers", () => {
    expect(parseHref("/sdlc?card=0").sdlc.card).toBeNull();
    expect(parseHref("/sdlc?card=-5").sdlc.card).toBeNull();
    expect(parseHref("/sdlc?card=abc").sdlc.card).toBeNull();
    expect(parseHref("/sdlc?card=12").sdlc.card).toBe(12);
  });

  it("never throws on arbitrary garbage input", () => {
    expect(() => parseHref("/e2e?%%%&run=&&=x&runsort=:::")).not.toThrow();
    expect(() => parseRoute({ path: "", search: "???" })).not.toThrow();
  });
});

describe("safe encoding of runtime identifiers (GLASS-NAV-03)", () => {
  it("round-trips reserved and Unicode characters byte-for-byte", () => {
    const id = "tenant/with space:colon?and#hash&amp=é/漢字";
    const state: RouteState = {
      ...DEFAULT_ROUTE,
      tab: "e2e",
      e2e: { ...DEFAULT_ROUTE.e2e, run: id, drawer: true },
    };
    const href = routeToHref(state);
    expect(parseHref(href).e2e.run).toBe(id);
  });

  it("does not corrupt neighbouring parameters", () => {
    const state: RouteState = {
      ...DEFAULT_ROUTE,
      tab: "e2e",
      e2e: {
        ...DEFAULT_ROUTE.e2e,
        query: "a&b=c",
        run: "x/y/z?w",
        drawer: true,
        exec: { ...DEFAULT_ROUTE.e2e.exec, logQuery: "d=e&f" },
      },
    };
    const parsed = parseHref(routeToHref(state));
    expect(parsed.e2e.query).toBe("a&b=c");
    expect(parsed.e2e.run).toBe("x/y/z?w");
    expect(parsed.e2e.exec.logQuery).toBe("d=e&f");
  });
});

describe("length limits (GLASS-NAV-02)", () => {
  it("drops an over-long single value", () => {
    const long = "x".repeat(ROUTE_LIMITS.value + 1);
    expect(parseHref(`/?bundle=${long}`).bundle).toBeNull();
    const ok = "y".repeat(ROUTE_LIMITS.value);
    expect(parseHref(`/?bundle=${ok}`).bundle).toBe(ok);
  });

  it("caps the number of scopes", () => {
    const many = Array.from(
      { length: ROUTE_LIMITS.scopes + 10 },
      (_, i) => `scope=s${String(i).padStart(3, "0")}`,
    ).join("&");
    expect(parseHref(`/sdlc?${many}`).sdlc.scopes.length).toBe(
      ROUTE_LIMITS.scopes,
    );
  });

  it("degrades an over-length URL to the tab-only view", () => {
    const huge = "q".repeat(ROUTE_LIMITS.value);
    const state: RouteState = {
      ...DEFAULT_ROUTE,
      tab: "sdlc",
      sdlc: {
        ...DEFAULT_ROUTE.sdlc,
        scopes: Array.from({ length: ROUTE_LIMITS.scopes }, () => huge),
      },
    };
    // Distinct long scopes would exceed the URL cap; serialization degrades.
    const distinct: RouteState = {
      ...state,
      sdlc: {
        ...state.sdlc,
        scopes: Array.from(
          { length: ROUTE_LIMITS.scopes },
          (_, i) => huge + String(i),
        ),
      },
    };
    const loc = serializeRoute(distinct);
    expect(loc.path).toBe("/sdlc");
    expect(loc.search).toBe("");
  });
});
