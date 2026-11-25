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
        nowMs={Date.parse("2026-11-02T00:00:00Z")}
        onNavigate={() => undefined}
        onRefresh={() => undefined}
      />
    </IntlProvider>,
  );
}

describe("DeliveryView promotion history (FLT-DLV-18)", () => {
  // Verifies: FLT-DLV-18.02
  it("flags a Passed group's failed attempt and lists what happened per environment", () => {
    renderRetried();
    expect(screen.getAllByText("1 failed attempt").length).toBeGreaterThan(0);
    const list = screen.getByRole("list", { name: "Promotions in int-hub" });
    const items = within(list).getAllByRole("listitem");
    expect(items.map((i) => i.textContent.replace(/\s+/g, " ").trim())).toEqual(
      [
        `Failed: ${identiName("sha256:aaaa")} (GitOps #2310), promoted 23:00 - Release analysis failed`,
        `Passed: ${identiName("sha256:bbbb")} (GitOps #2314), promoted 23:10`,
      ],
    );
    // A single attempt that passed adds nothing to the environment's status.
    expect(
      screen.queryByRole("list", { name: "Promotions in int-spoke" }),
    ).toBeNull();
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
        nowMs={Date.parse("2026-11-02T00:00:00Z")}
        onNavigate={() => undefined}
        onRefresh={() => undefined}
      />
    </IntlProvider>,
  );
}

describe("DeliveryView progress bar and Where to look (FLT-DLV-19, FLT-DLV-20)", () => {
  // Verifies: FLT-DLV-19.01
  it("shows one numbered cell per phase with its own label", () => {
    renderFailedBuild();
    const bar = screen.getByRole("list", {
      name: /Delivery progress of/,
    });
    const cells = within(bar).getAllByRole("listitem");
    expect(cells).toHaveLength(5);
    expect(cells[0]?.textContent).toMatch(/^1/);
    expect(cells[0]?.textContent).toMatch(/Failed/);
    expect(cells[4]?.textContent).toMatch(/^5/);
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
