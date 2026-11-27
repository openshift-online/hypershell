import { act, render, screen, within } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { describe, expect, it } from "vitest";

import { identiName } from "../../../domain/map/identiname";
import { mapDelivery } from "../../api/delivery-wire";
import { DeliveryView } from "./delivery-view";

const SHA = "2c6a1f0".padEnd(40, "0");
const NOW = Date.parse("2026-11-01T13:28:00Z");

// A busy moment: one merge rebuilt three components, two are in bundles, the
// int-hub analysis runs, and the agent bundle will restart it
// (FLT-DLV-22, FLT-DLV-23).
const BUSY = {
  enabled: true,
  stages: [
    { key: "merged", kind: "pipeline" },
    { key: "build", kind: "pipeline" },
    { key: "policy", kind: "pipeline" },
    { key: "release", kind: "pipeline" },
    { key: "gitops", kind: "pipeline" },
    { key: "env:int", kind: "group", group: "int" },
    { key: "env:stage", kind: "group", group: "stage" },
  ],
  changes: [
    {
      sha: SHA,
      shortSha: "2c6a1f0",
      pr: 999,
      mergedAt: "2026-11-01T12:31:00Z",
      title: "Gate cluster targeting",
      stages: [
        { key: "merged", state: "passed", finishedAt: "2026-11-01T12:31:00Z" },
        {
          key: "build",
          state: "passed",
          finishedAt: "2026-11-01T13:24:00Z",
          reason: "rebuilt-in",
          rebuiltIn: { sha: "e".repeat(40), shortSha: "eeeeeee", pr: 1000 },
        },
        { key: "policy", state: "passed", finishedAt: "2026-11-01T13:25:00Z" },
        {
          key: "release",
          state: "running",
          reason: "partly-released",
          partly: {
            done: 2,
            total: 3,
            missing: [{ component: "agent", state: "releasing" }],
          },
        },
        {
          key: "gitops",
          state: "running",
          reason: "partly-shipped",
          partly: { done: 2, total: 3 },
        },
        {
          key: "env:int",
          state: "running",
          reason: "gate-running",
          gate: "hypershell-analysis",
          startedAt: "2026-11-01T13:11:00Z",
          partly: { done: 2, total: 3 },
          restart: {
            components: ["agent"],
            envs: ["int-hub"],
            landsAt: "2026-11-01T13:40:00Z",
          },
        },
        {
          key: "env:stage",
          state: "waiting",
          reason: "waiting-envs",
          waitingFor: ["int-hub"],
        },
      ],
    },
  ],
  train: [
    {
      digest: "sha256:cccc",
      version: "v3",
      publishedAt: "2026-11-01T13:00:00Z",
      locked: true,
      lockedAt: "2026-11-01T13:06:00Z",
      same: true,
      changed: [],
      active: ["int-hub", "int-spoke"],
      proposed: ["stage-a"],
    },
    {
      inferred: true,
      builtAt: "2026-11-01T13:24:00Z",
      policyAt: "2026-11-01T13:25:00Z",
      expectedAt: "2026-11-01T13:30:00Z",
      changed: [{ component: "agent", revision: "2c6a1f0" }],
      restarts: ["int-hub"],
    },
  ],
  now: {
    appHead: {
      sha: SHA,
      shortSha: "2c6a1f0",
      pr: 999,
      title: "Gate cluster targeting",
      mergedAt: "2026-11-01T12:31:00Z",
    },
    gitopsHead: {
      digest: "sha256:cccc",
      version: "v3",
      publishedAt: "2026-11-01T13:00:00Z",
      lockedAt: "2026-11-01T13:06:00Z",
      pr: 2341,
    },
    checks: [],
    environments: [
      {
        env: "int-hub",
        group: "int",
        digest: "sha256:cccc",
        state: "running",
        gate: "hypershell-analysis",
        since: "2026-11-01T13:11:00Z",
        attempt: 1,
        attemptOf: 3,
      },
      {
        env: "int-spoke",
        group: "int",
        digest: "sha256:cccc",
        state: "passed",
        passedAt: "2026-11-01T13:17:00Z",
      },
      {
        env: "stage-a",
        group: "stage",
        digest: "sha256:bbbb",
        state: "waiting",
        proposedDigest: "sha256:cccc",
        waitingFor: ["int-hub"],
      },
    ],
    next: [
      {
        kind: "expecting-bundle",
        components: ["agent"],
        changes: [{ sha: SHA, shortSha: "2c6a1f0", pr: 999 }],
        at: "2026-11-01T13:30:00Z",
      },
      { kind: "restart", envs: ["int-hub"], at: "2026-11-01T13:40:00Z" },
      {
        kind: "promote",
        envs: ["stage-a"],
        after: ["int-hub"],
        at: "2026-11-01T14:05:00Z",
      },
    ],
  },
  sources: {},
};

function renderView(raw: unknown) {
  render(
    <IntlProvider locale="en" defaultLocale="en" timeZone="UTC">
      <DeliveryView
        state={{
          status: "ready",
          plane: {
            data: mapDelivery(raw),
            generatedAt: "2026-11-01T13:28:00Z",
            stale: false,
            error: null,
          },
        }}
        selectedSha={null}
        nowMs={NOW}
        onNavigate={() => undefined}
        onRefresh={() => undefined}
      />
    </IntlProvider>,
  );
}

function text(el: Element): string {
  return el.textContent.replace(/\s+/g, " ").trim();
}

/**
 * Matches the innermost element whose whole text is the given text: times are
 * their own elements, so a line's text spans several nodes (FLT-DLV-24).
 */
function line(want: string | RegExp) {
  const ok = (el: Element) =>
    typeof want === "string" ? text(el) === want : want.test(text(el));
  return (_: string, el: Element | null) =>
    el !== null && ok(el) && !Array.from(el.children).some(ok);
}

describe("Pipeline now (FLT-DLV-23)", () => {
  // Verifies: FLT-DLV-23.01, FLT-DLV-22.01, FLT-DLV-22.02
  it("states the heads, what is in flight, and what happens next", () => {
    renderView(BUSY);
    const calm = identiName("sha256:cccc");
    expect(
      screen.getByText(
        line("2c6a1f0 (#999) Gate cluster targeting, merged 12:31"),
      ),
    ).toBeTruthy();
    expect(
      screen.getByText(line(`Bundle ${calm}, published 13:00, locked 13:06`)),
    ).toBeTruthy();
    expect(
      screen.getByText(
        line(
          /agent bundle releasing \(inferred\) - built 13:24, policy check passed 13:25/,
        ),
      ),
    ).toBeTruthy();
    expect(
      screen.getByText(
        line(
          new RegExp(`Release analysis - 17 min, attempt 1 of 3, on ${calm}`),
        ),
      ),
    ).toBeTruthy();
    // FLT-DLV-23.04: a merged change whose bundle is not published yet.
    expect(
      screen.getByText(line("Expecting bundle for #999 - about 13:30")),
    ).toBeTruthy();
    expect(
      screen.getByText(
        line("Int promotes it; the int-hub gate restarts - about 13:40"),
      ),
    ).toBeTruthy();
    expect(
      screen.getByText(
        line("Stage promotes after int-hub passes - about 14:05"),
      ),
    ).toBeTruthy();

    const train = screen.getByRole("grid", { name: "Bundle train" });
    const rows = within(train).getAllByRole("row").slice(1).map(text);
    expect(rows[0]).toContain("Same components");
    expect(rows[0]).toContain(
      "Active in int-hub and int-spoke; proposed in stage-a",
    );
    expect(rows[1]).toContain("Releasing (inferred)");
    expect(rows[1]).toContain("Will restart the int-hub gate");

    const envs = screen.getByRole("grid", { name: "Environments now" });
    const envRows = within(envs).getAllByRole("row").slice(1).map(text);
    expect(envRows).toHaveLength(3);
    expect(envRows[1]).toContain("Gates passed 13:17");
    expect(envRows[2]).toContain("Waiting for int-hub");
  });

  // Verifies: FLT-DLV-23.02
  it("reads Nothing in flight on a quiet pipeline", () => {
    renderView({
      ...BUSY,
      changes: [],
      train: [],
      now: { ...BUSY.now, environments: [BUSY.now.environments[1]], next: [] },
    });
    expect(screen.getByText("Nothing in flight")).toBeTruthy();
    expect(screen.getByText("Nothing expected")).toBeTruthy();
    expect(screen.queryByRole("grid", { name: "Bundle train" })).toBeNull();
  });

  // Verifies: FLT-DLV-23.06
  it("names the bundle a re-cut GitOps head repeats", () => {
    renderView({
      ...BUSY,
      now: {
        ...BUSY.now,
        gitopsHead: { ...BUSY.now.gitopsHead, sameAs: "sha256:bbbb" },
      },
    });
    expect(
      screen.getByText(
        line(`Same as ${identiName("sha256:bbbb")} - no new code`),
      ),
    ).toBeTruthy();
  });

  // Verifies: FLT-DLV-23.06
  it("says nothing about re-cuts for a head with new code", () => {
    renderView(BUSY);
    expect(screen.queryByText(/no new code/)).toBeNull();
  });

  it("names the answers an unavailable source hides", () => {
    renderView({
      ...BUSY,
      sources: { gitops: { never: true, stale: true, error: "down" } },
    });
    expect(
      screen.getByText(/Cannot tell the heads and what happens next/),
    ).toBeTruthy();
  });
});

describe("Stage cells for trains and superseded builds (FLT-DLV-05, FLT-DLV-06, FLT-DLV-08, FLT-DLV-10)", () => {
  // Verifies: FLT-DLV-05.04, FLT-DLV-06.07, FLT-DLV-08.05, FLT-DLV-10.07
  it("shows rebuilt, partly shipped, waiting-for, and restart texts", () => {
    renderView(BUSY);
    const strip = screen.getByRole("list", {
      name: "Changes on main, newest first",
    });
    const cells = text(strip);
    expect(cells).toContain("Rebuilt in eeeeeee (#1000)");
    expect(cells).toContain(
      "2 of 3 components released - agent releasing (inferred)",
    );
    expect(cells).toContain("Partly: 2 of 3 components");
    expect(cells).toContain(
      "Release analysis running - restarts when the agent bundle lands",
    );
    expect(cells).toContain("Waiting for int-hub");
  });

  it("says Next restarts the gate when the newer bundle lands", () => {
    renderView(BUSY);
    expect(
      screen.getAllByText(
        line(
          /Restarts the int-hub release analysis when the agent bundle lands - about 13:40/,
        ),
      ).length,
    ).toBeGreaterThan(0);
  });
});

describe("Pipeline now Renovate waits (FLT-DLV-23)", () => {
  // Verifies: FLT-DLV-23.05, FLT-DLV-24.02
  it("names the bundle with its identicon, its Renovate state, and the lock time", () => {
    renderView({
      ...BUSY,
      now: {
        ...BUSY.now,
        next: [
          {
            kind: "renovate",
            digest: "sha256:dddd",
            version: "v4",
            state: "running",
            pr: 2951,
            at: "2026-11-01T13:34:00Z",
          },
        ],
      },
    });
    const alias = identiName("sha256:dddd");
    const item = screen.getByText(
      line(`${alias} - PR #2951 open, checks running - locked about 13:34`),
    );
    expect(
      item.querySelector('[data-identicon-seed="sha256:dddd"]'),
    ).not.toBeNull();
  });
});

describe("Times and bundle identity (FLT-DLV-24)", () => {
  // Verifies: FLT-DLV-24.01
  it("marks up each shown time with its ISO value and reveals the full date on focus", async () => {
    renderView(BUSY);
    const head = screen.getByText(
      line("2c6a1f0 (#999) Gate cluster targeting, merged 12:31"),
    );
    const time = head.querySelector("time");
    expect(time?.getAttribute("dateTime")).toBe("2026-11-01T12:31:00.000Z");
    expect(time?.getAttribute("tabindex")).toBe("0");
    act(() => {
      time?.focus();
    });
    expect((await screen.findByRole("tooltip")).textContent).toMatch(
      /Sun, 1 Nov 2026, 12:31:00 UTC|Sun, Nov 1, 2026, 12:31:00 UTC/,
    );
  });
});
