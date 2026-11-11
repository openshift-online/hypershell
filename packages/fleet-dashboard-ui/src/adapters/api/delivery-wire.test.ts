import { describe, expect, it } from "vitest";

import { mapDelivery } from "./delivery-wire";

describe("mapDelivery promotion history (FLT-DLV-18)", () => {
  // Verifies: FLT-DLV-18.01
  it("maps each environment's attempts and the group's failed attempts", () => {
    const data = mapDelivery({
      enabled: true,
      changes: [
        {
          sha: "a".repeat(40),
          stages: [{ key: "env:int", state: "passed", failedAttempts: 1 }],
          environments: [
            {
              env: "int-hub",
              group: "int",
              state: "passed",
              contains: true,
              history: [
                {
                  version: "v1",
                  digest: "sha256:aaaa",
                  gitopsPr: 2310,
                  promotedAt: "2026-11-01T23:00:00Z",
                  url: "https://example.test/pull/1",
                  outcome: "replaced",
                  gate: "hypershell-analysis",
                },
                {
                  version: "v2",
                  digest: "sha256:bbbb",
                  promotedAt: "2026-11-01T23:10:00Z",
                  outcome: "passed",
                },
                { promotedAt: "2026-11-01T23:20:00Z", outcome: "later" },
              ],
            },
            { env: "int-spoke", group: "int", state: "passed" },
          ],
        },
      ],
      sources: { history: { stale: true, error: "rate limited" } },
    });
    const change = data.changes[0];
    expect(change?.stages[0]?.failedAttempts).toBe(1);
    expect(change?.environments[0]?.history).toEqual([
      {
        version: "v1",
        digest: "sha256:aaaa",
        gitopsPr: 2310,
        promotedAt: "2026-11-01T23:00:00Z",
        url: "https://example.test/pull/1",
        outcome: "replaced",
        gate: "hypershell-analysis",
      },
      {
        version: "v2",
        digest: "sha256:bbbb",
        gitopsPr: null,
        promotedAt: "2026-11-01T23:10:00Z",
        url: null,
        outcome: "passed",
        gate: null,
      },
      // An unrecognized outcome reads as Unknown, never as Passed.
      expect.objectContaining({ outcome: "unknown" }) as unknown,
    ]);
    expect(change?.environments[1]?.history).toEqual([]);
    expect(data.sources.history).toMatchObject({
      stale: true,
      error: "rate limited",
    });
  });
});
