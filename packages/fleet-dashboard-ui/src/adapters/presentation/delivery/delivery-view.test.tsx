import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { describe, expect, it } from "vitest";

import { identiName } from "../../../domain/map/identiname";
import { mapDelivery } from "../../api/delivery-wire";
import { DeliveryView } from "./delivery-view";

const SHA = "5d2e8c1".padEnd(40, "0");

function renderRetried() {
  const data = mapDelivery({
    enabled: true,
    stages: [{ key: "env:int", kind: "group", group: "int" }],
    changes: [
      {
        sha: SHA,
        shortSha: "5d2e8c1",
        mergedAt: "2026-11-01T22:40:00Z",
        title: "Retried change",
        delivered: true,
        stages: [
          {
            key: "env:int",
            state: "passed",
            finishedAt: "2026-11-01T23:10:00Z",
            failedAttempts: 1,
          },
        ],
        environments: [
          {
            env: "int-hub",
            group: "int",
            state: "passed",
            contains: true,
            history: [
              {
                version: "brisk falcon",
                digest: "sha256:aaaa",
                gitopsPr: 2310,
                promotedAt: "2026-11-01T23:00:00Z",
                outcome: "failed",
                gate: "hypershell-analysis",
              },
              {
                version: "calm meadow",
                digest: "sha256:bbbb",
                gitopsPr: 2314,
                promotedAt: "2026-11-01T23:10:00Z",
                outcome: "passed",
              },
            ],
          },
          {
            env: "int-spoke",
            group: "int",
            state: "passed",
            contains: true,
            history: [
              {
                version: "brisk falcon",
                digest: "sha256:aaaa",
                promotedAt: "2026-11-01T23:00:00Z",
                outcome: "passed",
              },
            ],
          },
        ],
      },
    ],
  });
  render(
    <IntlProvider locale="en" defaultLocale="en" timeZone="UTC">
      <DeliveryView
        state={{
          status: "ready",
          plane: {
            data,
            generatedAt: "2026-11-02T00:00:00Z",
            stale: false,
            error: null,
          },
        }}
        selectedSha={SHA}
        nowMs={Date.parse("2026-11-01T23:30:00Z")}
        onNavigate={() => undefined}
        onRefresh={() => undefined}
      />
    </IntlProvider>,
  );
}

/** Every rendered environment line, whitespace-normalised. */
function envLines(): string[] {
  return Array.from(document.querySelectorAll("li, dd")).map((e) =>
    e.textContent.replace(/\s+/g, " ").trim(),
  );
}

function renderEnv(
  history: Record<string, unknown>[],
  onNavigate: (action: unknown) => void = () => undefined,
  shipped?: React.ReactNode,
) {
  const data = mapDelivery({
    enabled: true,
    stages: [{ key: "env:int", kind: "group", group: "int" }],
    changes: [
      {
        sha: SHA,
        shortSha: "5d2e8c1",
        mergedAt: "2026-11-01T22:40:00Z",
        title: "One environment",
        delivered: true,
        bundle: {
          version: "v1",
          digest: "sha256:aaaa",
          tag: "t",
          locked: true,
        },
        stages: [{ key: "env:int", state: "passed" }],
        environments: [
          {
            env: "int-hub",
            group: "int",
            state: "passed",
            contains: true,
            history,
          },
        ],
      },
    ],
  });
  render(
    <IntlProvider locale="en" defaultLocale="en" timeZone="UTC">
      <DeliveryView
        state={{
          status: "ready",
          plane: {
            data,
            generatedAt: "2026-11-02T00:00:00Z",
            stale: false,
            error: null,
          },
        }}
        selectedSha={SHA}
        nowMs={Date.parse("2026-11-01T23:30:00Z")}
        onNavigate={onNavigate}
        shipped={shipped}
        onRefresh={() => undefined}
      />
    </IntlProvider>,
  );
}

describe("DeliveryView promotion history (FLT-DLV-18)", () => {
  // Verifies: FLT-DLV-18.02
  it("flags a Passed group's failed attempt and lists only the first bundle's promotions", () => {
    renderRetried();
    expect(screen.getAllByText("1 failed attempt").length).toBeGreaterThan(0);
    const list = screen.getByRole("list", { name: "Promotions in int-hub" });
    const items = within(list).getAllByRole("listitem");
    expect(items.map((i) => i.textContent.replace(/\s+/g, " ").trim())).toEqual(
      [
        `Failed: ${identiName("sha256:aaaa")} (GitOps #2310), promoted 23:00 - Release analysis failed`,
      ],
    );
    // The later bundle that passed says nothing new about the change.
    expect(envLines().some((l) => l.includes(identiName("sha256:bbbb")))).toBe(
      false,
    );
    // A single attempt that passed adds nothing to the environment's status.
    expect(
      screen.queryByRole("list", { name: "Promotions in int-spoke" }),
    ).toBeNull();
  });

  // Verifies: FLT-DLV-18.01
  it("reads Not deployed, naming the bundle that replaced the first one", () => {
    renderEnv([
      {
        version: "v1",
        digest: "sha256:aaaa",
        promotedAt: "2026-11-01T23:00:00Z",
        outcome: "replaced",
        gate: "hypershell-analysis",
      },
      {
        version: "v2",
        digest: "sha256:bbbb",
        promotedAt: "2026-11-01T23:10:00Z",
        outcome: "passed",
        gatesPassedAt: "2026-11-01T23:20:00Z",
      },
    ]);
    const later = identiName("sha256:bbbb");
    expect(envLines()).toContain(
      `int-hub: Not deployed - replaced by ${later} 23:10`,
    );
    expect(
      document.querySelector(`[data-identicon-seed="sha256:bbbb"]`),
    ).not.toBeNull();
    // The later bundle's pass never shows as the change's own.
    expect(
      envLines().filter(
        (l) =>
          l.startsWith("int-hub") &&
          (l.includes("Gates passed") || l.includes("Passed")),
      ),
    ).toEqual([]);
  });

  // Verifies: FLT-DLV-19.04
  it("lists environments only on their group step, each linked to the Fleet Map", () => {
    const actions: unknown[] = [];
    renderEnv(
      [
        {
          version: "v1",
          digest: "sha256:aaaa",
          promotedAt: "2026-11-01T23:00:00Z",
          outcome: "passed",
          gatesPassedAt: "2026-11-01T23:20:00Z",
        },
      ],
      (a) => actions.push(a),
    );
    const link = screen.getByRole("button", {
      name: "Open int-hub on Fleet Map",
    });
    expect(link.closest("li")?.closest("ol")).not.toBeNull();
    fireEvent.click(link);
    expect(actions).toEqual([{ type: "openNodeOnMap", id: "int-hub" }]);
    expect(screen.queryByRole("heading", { name: "Environments" })).toBeNull();
    expect(
      screen.getAllByRole("button", { name: "Open int-hub on Fleet Map" }),
    ).toHaveLength(1);
  });

  // Verifies: FLT-DLV-18.04
  it("shows the deployed first bundle with its identicon and times", () => {
    renderEnv([
      {
        version: "v1",
        digest: "sha256:aaaa",
        promotedAt: "2026-11-01T23:00:00Z",
        outcome: "passed",
        gatesPassedAt: "2026-11-01T23:20:00Z",
      },
    ]);
    const first = identiName("sha256:aaaa");
    expect(envLines()).toContain(
      `int-hub: Passed - ${first} deployed 23:00 - Gates passed 23:20`,
    );
    expect(
      document.querySelectorAll(`[data-identicon-seed="sha256:aaaa"]`).length,
    ).toBeGreaterThan(0);
  });
});

const FAILED_SHA = "9a1b2c3".padEnd(40, "0");

function renderFailedBuild() {
  const data = mapDelivery({
    enabled: true,
    stages: [
      { key: "merged", kind: "pipeline" },
      { key: "build", kind: "pipeline" },
      { key: "policy", kind: "pipeline" },
      { key: "release", kind: "pipeline" },
      { key: "gitops", kind: "pipeline" },
      { key: "env:int", kind: "group", group: "int" },
    ],
    changes: [
      {
        sha: FAILED_SHA,
        shortSha: "9a1b2c3",
        mergedAt: "2026-11-01T23:00:00Z",
        title: "Failed change",
        stages: [
          { key: "merged", state: "passed" },
          {
            key: "build",
            state: "failed",
            components: ["web"],
            finishedAt: "2026-11-01T23:20:00Z",
          },
          { key: "policy", state: "not-started" },
          { key: "release", state: "not-started" },
          { key: "gitops", state: "not-started" },
          { key: "env:int", state: "not-started" },
        ],
        builds: [
          {
            component: "web",
            state: "failed",
            completedAt: "2026-11-01T23:20:00Z",
            url: "https://example.test/run/1",
          },
        ],
        imageCheck: {
          state: "failed",
          url: "https://example.test/job/1",
          failures: ["No usable image for: web."],
        },
      },
    ],
  });
  render(
    <IntlProvider locale="en" defaultLocale="en" timeZone="UTC">
      <DeliveryView
        state={{
          status: "ready",
          plane: {
            data,
            generatedAt: "2026-11-02T00:00:00Z",
            stale: false,
            error: null,
          },
        }}
        selectedSha={null}
        nowMs={Date.parse("2026-11-01T23:30:00Z")}
        onNavigate={() => undefined}
        onRefresh={() => undefined}
      />
    </IntlProvider>,
  );
}

describe("DeliveryView progress bar and Where to look (FLT-DLV-19, FLT-DLV-20)", () => {
  // Verifies: FLT-DLV-19.01
  it("shows one numbered step per stage with its state and time", () => {
    renderFailedBuild();
    const bar = screen.getByRole("list", {
      name: /Delivery progress of/,
    });
    const steps = Array.from(bar.querySelectorAll(":scope > li"));
    expect(steps).toHaveLength(6);
    expect(steps[0]?.textContent).toMatch(/^1Merged/);
    expect(steps[1]?.textContent).toMatch(/^2Build/);
    expect(steps[1]?.textContent).toMatch(/Failed/);
    // FLT-DLV-24: the failure time is a time element.
    expect(steps[1]?.querySelector("time")?.textContent).toBe("23:20");
    expect(steps[5]?.textContent).toMatch(/^6Int/);
  });

  // Verifies: FLT-DLV-20.02
  it("opens the failed-build popup on focus and keeps it open on the click that follows", async () => {
    renderFailedBuild();
    const bar = screen.getByRole("list", { name: /Delivery progress of/ });
    const trigger = within(bar).getByRole("button", { name: "Where to look" });
    act(() => {
      trigger.focus();
    });
    fireEvent.click(trigger);
    const dialog = await screen.findByRole("dialog");
    expect(dialog.textContent).toContain(
      "Re-run the failed pipeline, or merge a change to the same component",
    );
    expect(
      within(dialog)
        .getByRole("link", { name: /View check run/ })
        .getAttribute("href"),
    ).toBe("https://example.test/run/1");
  });

  // Verifies: FLT-DLV-20.03
  it("shows the failed image check and its annotation in the Build popup", async () => {
    renderFailedBuild();
    const bar = screen.getByRole("list", { name: /Delivery progress of/ });
    fireEvent.click(within(bar).getByRole("button", { name: "Where to look" }));
    const dialog = await screen.findByRole("dialog");
    expect(dialog.textContent).toContain("Image check");
    expect(dialog.textContent).toContain("No usable image for: web.");
    expect(
      within(dialog)
        .getByRole("link", { name: /View image check/ })
        .getAttribute("href"),
    ).toBe("https://example.test/job/1");
  });
});

const NEW_SHA = "b81f3e2".padEnd(40, "0");
const OLD_SHA = "c53fb9b".padEnd(40, "0");

function renderStrip(selectedSha: string | null) {
  const pipeline = (gitops: Record<string, unknown>) => [
    { key: "merged", state: "passed", finishedAt: "2026-11-25T18:42:00Z" },
    { key: "build", state: "passed", finishedAt: "2026-11-25T18:58:00Z" },
    { key: "policy", state: "passed", finishedAt: "2026-11-25T19:02:00Z" },
    { key: "release", state: "passed", finishedAt: "2026-11-25T19:09:00Z" },
    gitops,
  ];
  const data = mapDelivery({
    enabled: true,
    stages: [
      { key: "merged", kind: "pipeline" },
      { key: "build", kind: "pipeline" },
      { key: "policy", kind: "pipeline" },
      { key: "release", kind: "pipeline" },
      { key: "gitops", kind: "pipeline" },
    ],
    changes: [
      {
        sha: NEW_SHA,
        shortSha: "b81f3e2",
        pr: 1192,
        mergedAt: "2026-11-25T18:42:00Z",
        title: "Newest change",
        stages: pipeline({
          key: "gitops",
          state: "running",
          pr: 2951,
          checks: "running",
          startedAt: "2026-11-25T19:10:00Z",
        }),
        bundle: {
          tag: "release-bundle-v20261125",
          version: "v20261125",
          digest: "sha256:eeee",
          publishedAt: "2026-11-25T19:09:00Z",
          locked: false,
        },
      },
      {
        sha: OLD_SHA,
        shortSha: "c53fb9b",
        pr: 995,
        mergedAt: "2026-11-25T18:42:00Z",
        title: "Older change",
        delivered: true,
        deliveredAt: "2026-11-25T20:56:00Z",
        stages: pipeline({
          key: "gitops",
          state: "passed",
          finishedAt: "2026-11-25T19:11:00Z",
        }),
        bundle: {
          tag: "release-bundle-v20261124",
          version: "v20261124",
          digest: "sha256:ffff",
          publishedAt: "2026-11-25T19:09:00Z",
          locked: true,
        },
      },
    ],
  });
  render(
    <IntlProvider locale="en" defaultLocale="en" timeZone="UTC">
      <DeliveryView
        state={{
          status: "ready",
          plane: {
            data,
            generatedAt: "2026-11-25T19:12:00Z",
            stale: false,
            error: null,
          },
        }}
        selectedSha={selectedSha}
        nowMs={Date.parse("2026-11-25T19:12:00Z")}
        onNavigate={() => undefined}
        onRefresh={() => undefined}
      />
    </IntlProvider>,
  );
}

function panelOf(name: string): HTMLElement {
  const strip = screen.getByRole("list", {
    name: "Changes on main, newest first",
  });
  const panel = within(strip).getByRole("listitem", { name });
  return panel;
}

describe("DeliveryView change panels (FLT-DLV-03, FLT-DLV-07, FLT-DLV-11)", () => {
  // Verifies: FLT-DLV-03.03
  it("lays out one panel per change, newest first, in a keyboard-scrollable strip", () => {
    renderStrip(null);
    const strip = screen.getByRole("list", {
      name: "Changes on main, newest first",
    });
    expect(strip.getAttribute("tabindex")).toBe("0");
    const panels = Array.from(strip.querySelectorAll(":scope > li"));
    expect(panels.map((p) => p.getAttribute("aria-labelledby"))).toEqual([
      `delivery-change-${NEW_SHA}`,
      `delivery-change-${OLD_SHA}`,
    ]);
  });

  // Verifies: FLT-DLV-07.01
  it("shows the Renovate pull request, its checks, and when it opened", () => {
    renderStrip(null);
    const step = within(panelOf("b81f3e2 (#1192)"))
      .getByRole("list", { name: /Delivery progress of/ })
      .querySelectorAll(":scope > li")[4];
    expect(step?.textContent.replace(/\s+/g, " ")).toContain(
      "PR #2951, checks running - opened 19:10",
    );
  });

  // Verifies: FLT-DLV-11.04, FLT-DLV-24.02
  it("gives every bundle fact a time and names the bundle with its identicon", () => {
    renderStrip(null);
    const panel = panelOf("c53fb9b (#995)");
    const text = panel.textContent.replace(/\s+/g, " ");
    expect(text).toContain("v20261124 - published 19:09");
    expect(text).toContain("Locked 19:11");
    expect(text).toContain(identiName("sha256:ffff"));
    expect(
      panel.querySelector('img[data-identicon-seed="sha256:ffff"]'),
    ).not.toBeNull();
  });

  // Verifies: FLT-DLV-10.02
  it("states when a delivered change was delivered and how long it took", () => {
    renderStrip(null);
    expect(panelOf("c53fb9b (#995)").textContent).toContain(
      "Delivered 20:56 - 2 h 14 min from merge",
    );
  });

  // Verifies: FLT-DLV-14.01
  it("marks the panel the change parameter selects", () => {
    renderStrip(OLD_SHA);
    expect(panelOf("c53fb9b (#995)").getAttribute("data-selected")).toBe(
      "true",
    );
    expect(panelOf("b81f3e2 (#1192)").getAttribute("data-selected")).toBeNull();
  });
});

const SHIPPED_LABEL = "Shipped";

describe("DeliveryView layout (FLT-DLV-25)", () => {
  // Verifies: FLT-DLV-01.01, FLT-DLV-25.01
  it("draws Shipped between Pipeline now and Changes on main", () => {
    renderEnv([], () => undefined, <section aria-label={SHIPPED_LABEL} />);
    const now = screen.getByText("Pipeline now");
    const shipped = screen.getByRole("region", { name: SHIPPED_LABEL });
    const strip = screen.getByRole("list", {
      name: "Changes on main, newest first",
    });
    expect(
      now.compareDocumentPosition(shipped) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(
      shipped.compareDocumentPosition(strip) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it("offers no bundle buttons on a change panel", () => {
    renderEnv([]);
    expect(screen.queryByRole("button", { name: /Show bundle/ })).toBeNull();
  });
});
