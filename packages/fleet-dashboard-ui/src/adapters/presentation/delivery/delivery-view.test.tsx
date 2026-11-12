import { render, screen, within } from "@testing-library/react";
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
