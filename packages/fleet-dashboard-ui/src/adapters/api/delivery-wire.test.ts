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
                  gatesPassedAt: "2026-11-01T23:30:00Z",
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
        gatesPassedAt: null,
      },
      {
        version: "v2",
        digest: "sha256:bbbb",
        gitopsPr: null,
        promotedAt: "2026-11-01T23:10:00Z",
        url: null,
        outcome: "passed",
        gate: null,
        gatesPassedAt: "2026-11-01T23:30:00Z",
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

describe("mapDelivery estimates and diagnostics (FLT-DLV-12)", () => {
  // Verifies: FLT-DLV-09.01, FLT-DLV-12
  it("maps stage estimates to milliseconds and drops invalid ones", () => {
    const data = mapDelivery({
      enabled: true,
      changes: [
        {
          sha: "a".repeat(40),
          stages: [
            {
              key: "build",
              state: "running",
              estimateSeconds: 600,
              slowestSeconds: 840,
            },
            { key: "policy", state: "not-started", estimateSeconds: -1 },
          ],
          builds: [
            { component: "board", state: "passed", outsideBundle: true },
          ],
        },
      ],
    });
    const change = data.changes[0];
    expect(change?.stages[0]).toMatchObject({
      estimateMs: 600_000,
      slowestMs: 840_000,
    });
    expect(change?.stages[1]).toMatchObject({
      estimateMs: null,
      slowestMs: null,
    });
    expect(change?.builds[0]?.outsideBundle).toBe(true);
    expect(change?.imageCheck).toBeNull();
    expect(data.releaseDiagnostics).toBeNull();
  });

  // Verifies: FLT-DLV-20.03
  it("maps the image check with its failure annotations", () => {
    const data = mapDelivery({
      enabled: true,
      changes: [
        {
          sha: "b".repeat(40),
          imageCheck: {
            state: "failed",
            url: "https://example.test/job",
            failures: ["No usable image for: web-console.", ""],
          },
        },
      ],
    });
    expect(data.changes[0]?.imageCheck).toEqual({
      state: "failed",
      url: "https://example.test/job",
      failures: ["No usable image for: web-console."],
    });
  });

  // Verifies: FLT-DLV-20.01
  it("maps the release diagnostics", () => {
    const data = mapDelivery({
      enabled: true,
      releaseDiagnostics: {
        lastGood: { tag: "t1", version: "v1", digest: "sha256:aaaa" },
        logUrl: "https://example.test/log",
        commits: [
          {
            sha: "b".repeat(40),
            title: "fix(release): read the snapshot",
            pr: 77,
            url: "https://example.test/commit/b",
            mergedAt: "2026-11-01T11:00:00Z",
          },
        ],
        registryTag: "t2",
        githubReleaseTag: "t1",
      },
    });
    expect(data.releaseDiagnostics).toMatchObject({
      lastGood: { digest: "sha256:aaaa" },
      logUrl: "https://example.test/log",
      appUrl: null,
      commits: [{ shortSha: "bbbbbbb", pr: 77 }],
      registryTag: "t2",
      githubReleaseTag: "t1",
    });
  });
});

describe("mapDelivery train and now (FLT-DLV-12, FLT-DLV-22, FLT-DLV-23)", () => {
  it("maps the bundle train, the Pipeline now data, and new stage fields", () => {
    const d = mapDelivery({
      enabled: true,
      changes: [
        {
          sha: "a".repeat(40),
          stages: [
            {
              key: "release",
              state: "running",
              partly: {
                done: 1,
                total: 2,
                missing: [{ component: "agent", state: "bogus" }],
              },
            },
          ],
        },
      ],
      train: [{ inferred: true, changed: [{ component: "agent" }] }],
      now: {
        environments: [{ env: "hub", state: "bogus", attempt: 0 }],
        next: [{ kind: "unknown-kind" }, { kind: "locked", at: "x" }],
      },
    });
    const stage = d.changes[0]?.stages[0];
    // An unrecognized missing state never reads as releasing.
    expect(stage?.partly?.missing[0]?.state).toBe("building");
    expect(stage?.restart).toBeNull();
    expect(d.train[0]?.inferred).toBe(true);
    expect(d.train[0]?.active).toEqual([]);
    expect(d.now.appHead).toBeNull();
    expect(d.now.environments[0]?.state).toBe("unknown");
    expect(d.now.environments[0]?.attempt).toBeNull();
    // Unknown next event kinds are dropped.
    expect(d.now.next.map((e) => e.kind)).toEqual(["locked"]);
  });

  it("defaults to an empty train and Pipeline now", () => {
    const d = mapDelivery({ enabled: true });
    expect(d.train).toEqual([]);
    expect(d.now.environments).toEqual([]);
    expect(d.now.next).toEqual([]);
  });
});
