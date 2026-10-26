import { describe, expect, it } from "vitest";

import {
  environmentGateBadge,
  governingInstance,
  orderedEnvironments,
} from "../../domain/promotion";
import {
  mapE2eRunDetail,
  mapE2eRunLogPage,
  mapE2eRuns,
  mapE2eStreamEvent,
  mapE2eStreamLog,
  mapFleet,
  mapInstances,
  mapPromotion,
  mapSdlc,
  mapSdlcAgents,
  mapTopology,
} from "./wire";

// Fixtures use deliberately fictional environment/instance names (alpha/beta/
// gamma) and a fake release id. The fleet-dashboard source is public, so real
// fleet identifiers must never appear in it -- not even in tests
// (data-architecture.spec §3.5, enforced by scripts/check_fleet_dashboard_firewall.sh).
describe("mapPromotion", () => {
  // A payload shaped exactly like the BFF/promotion.json wire contract (§5).
  const wire = {
    order: ["alpha", "beta"],
    environments: {
      beta: {
        active: { version: "abc1234", sha: "abc1234def" },
        proposed: { version: "abc1234", sha: "abc1234def" },
        activeGates: [
          { key: "hypershell-analysis", phase: "success" },
          { key: "argocd-health", phase: "success" },
        ],
        proposedGates: [{ key: "dependents-successful", phase: "success" }],
      },
      alpha: {
        active: { version: "abc1234" },
        proposed: { version: "abc1234" },
        activeGates: [{ key: "argocd-health", phase: "success" }],
        proposedGates: [],
      },
    },
    releases: { abc1234: { version: "abc1234" } },
    frontier: { version: "abc1234" },
  };

  it("projects the wire shape into the domain model", () => {
    const data = mapPromotion(wire);
    expect(data.order).toEqual(["alpha", "beta"]);
    const beta = data.environments.beta;
    if (!beta) throw new Error("expected beta environment");
    expect(beta.name).toBe("beta");
    expect(beta.activeRelease).toBe("abc1234");
    expect(beta.gates.map((g) => g.name)).toEqual([
      "hypershell-analysis",
      "argocd-health",
    ]);
    expect(data.releases).toEqual(["abc1234"]);
  });

  it("produces a model the domain helpers consume without throwing", () => {
    // This is the regression guard: the un-mapped wire object has `activeGates`
    // but no `gates`, so governingInstance/environmentGateBadge threw at render.
    const data = mapPromotion(wire);
    for (const env of orderedEnvironments(data)) {
      expect(() => governingInstance(env)).not.toThrow();
      expect(() => environmentGateBadge(env)).not.toThrow();
    }
    expect(orderedEnvironments(data).map((e) => e.name)).toEqual([
      "alpha",
      "beta",
    ]);
  });

  it("maps syncRetries, preserving a genuine 0 and null for absent data", () => {
    const data = mapPromotion({
      order: ["zero", "none"],
      environments: {
        zero: { active: { version: "v1" }, syncRetries: 0 },
        none: { active: { version: "v1" } },
      },
    });
    expect(data.environments.zero?.syncRetries).toBe(0);
    expect(data.environments.none?.syncRetries).toBeNull();
  });

  it("tolerates missing/empty fields", () => {
    const data = mapPromotion({});
    expect(data.order).toEqual([]);
    expect(data.environments).toEqual({});
    expect(data.releases).toEqual([]);
  });

  it("threads capabilities through unchanged, empty when absent", () => {
    // FLT-CAP-01.01 / 01.02: the wire set is carried verbatim and in order; an
    // environment that omits it carries an empty set rather than erroring.
    const data = mapPromotion({
      order: ["withCaps", "noCaps"],
      environments: {
        withCaps: {
          active: { version: "v1" },
          capabilities: [
            "provider/aws",
            "runtime/kata",
            "gateway-version/v0.1.2-rhaiv.7",
          ],
        },
        noCaps: { active: { version: "v1" } },
      },
    });
    expect(data.environments.withCaps?.capabilities).toEqual([
      "provider/aws",
      "runtime/kata",
      "gateway-version/v0.1.2-rhaiv.7",
    ]);
    expect(data.environments.noCaps?.capabilities).toEqual([]);
  });

  it("drops empty/non-string capability entries", () => {
    const data = mapPromotion({
      order: ["x"],
      environments: {
        x: {
          active: { version: "v1" },
          capabilities: ["runtime/kata", "", 7, null],
        },
      },
    });
    expect(data.environments.x?.capabilities).toEqual(["runtime/kata"]);
  });

  it("prefers the bundle digest over the gitops SHA for release identity", () => {
    const data = mapPromotion({
      order: ["delta"],
      environments: {
        delta: {
          active: {
            version: "v20260930",
            sha: "aaaaaaaa",
            digest: "sha256:dig",
          },
          proposed: {
            version: "v20260930",
            sha: "bbbbbbbb",
            digest: "sha256:dig",
          },
          activeGates: [],
        },
      },
      releases: {
        "sha256:dig": {
          version: "v20260930",
          sha: "aaaaaaaa",
          digest: "sha256:dig",
        },
      },
    });
    const delta = data.environments.delta;
    if (!delta) throw new Error("expected delta environment");
    // Two envs on the same bundle collapse: active/proposed point at the digest,
    // not the differing per-env gitops SHAs.
    expect(delta.activeDigest).toBe("sha256:dig");
    expect(delta.proposedDigest).toBe("sha256:dig");
    expect(data.releaseByDigest["sha256:dig"]?.digest).toBe("sha256:dig");
  });

  it("falls back to the SHA as digest when no bundle digest is present", () => {
    const data = mapPromotion({
      order: ["epsilon"],
      environments: {
        epsilon: {
          active: { version: "shortsha", sha: "cccccccc" },
          activeGates: [],
        },
      },
    });
    const epsilon = data.environments.epsilon;
    if (!epsilon) throw new Error("expected epsilon environment");
    expect(epsilon.activeDigest).toBe("cccccccc");
  });

  it("maps absent release versions to null", () => {
    const data = mapPromotion({
      order: ["gamma"],
      environments: { gamma: { activeGates: [] } },
    });
    const gamma = data.environments.gamma;
    if (!gamma) throw new Error("expected gamma environment");
    expect(gamma.activeRelease).toBeNull();
    expect(gamma.proposedRelease).toBeNull();
    expect(gamma.gates).toEqual([]);
  });
});

describe("mapFleet", () => {
  it("flattens the keyed wire map into a sorted instance list", () => {
    const data = mapFleet({
      zeta: { gateways: { running: 2 }, gatewaysTotal: 2 },
      alpha: { instance: "alpha", gatewaysTotal: 5 },
    });
    expect(data.instances.map((i) => i.instance)).toEqual(["alpha", "zeta"]);
  });

  it("defaults rate triples, history and nullable gauges", () => {
    const data = mapFleet({ a: {} });
    const inst = data.instances[0];
    expect(inst?.instance).toBe("a");
    expect(inst?.rpc).toEqual({ rate: 0, errorPct: 0, p95Ms: 0 });
    expect(inst?.gatewayHistory).toEqual([]);
    expect(inst?.managedClusters).toBeNull();
    expect(inst?.role).toBeNull();
    expect(inst?.sandboxes).toBe(0);
    expect(inst?.sandboxesByCluster).toEqual([]);
  });

  it("carries metrics, totals and history through", () => {
    const data = mapFleet({
      a: {
        gateways: { running: 3, failed: 1 },
        gatewaysTotal: 4,
        users: 12,
        logins: 5,
        userHistory: [10, 11, 12],
        loginsHistory: [3, 4, 5],
        historyTimes: [100, 200, 300],
        rpc: { rate: 2, errorPct: 0.5, p95Ms: 40 },
        provisionP95Ms: 120,
        gatewayHistory: [
          { running: 1 },
          { running: 2, provisioning: 1 },
          { failed: 4 },
        ],
        sandboxes: 9,
        sandboxesByCluster: [
          { managedCluster: "c2", count: 6 },
          { managedCluster: "c1", count: 3 },
          // A row with no managed-cluster label is dropped (identity-less, unplottable).
          { count: 2 },
        ],
        gatewaysByCluster: [
          { managedCluster: "c1", gateways: { running: 2 }, total: 2 },
          // A row with no managed-cluster label is dropped (unattributable).
          { gateways: { running: 5 }, total: 5 },
        ],
      },
    });
    const inst = data.instances[0];
    expect(inst?.gatewaysTotal).toBe(4);
    expect(inst?.users).toBe(12);
    expect(inst?.logins).toBe(5);
    expect(inst?.userHistory).toEqual([10, 11, 12]);
    expect(inst?.loginsHistory).toEqual([3, 4, 5]);
    expect(inst?.historyTimes).toEqual([100, 200, 300]);
    expect(inst?.rpc.p95Ms).toBe(40);
    expect(inst?.provisionP95Ms).toBe(120);
    expect(inst?.sandboxes).toBe(9);
    expect(inst?.sandboxesByCluster).toEqual([
      { managedCluster: "c2", count: 6 },
      { managedCluster: "c1", count: 3 },
    ]);
    expect(inst?.gatewaysByCluster).toEqual([
      { managedCluster: "c1", gateways: { running: 2 }, total: 2 },
    ]);
    // Missing phase fields default to 0 (num()), so every sample is fully shaped.
    expect(inst?.gatewayHistory).toEqual([
      { running: 1, provisioning: 0, failed: 0 },
      { running: 2, provisioning: 1, failed: 0 },
      { running: 0, provisioning: 0, failed: 4 },
    ]);
  });

  it("tolerates a null/empty payload", () => {
    expect(mapFleet({}).instances).toEqual([]);
    expect(mapFleet(null).instances).toEqual([]);
  });
});

describe("mapInstances", () => {
  it("expands bare instance names into summaries", () => {
    const data = mapInstances({ instances: ["alpha", "beta"] });
    expect(data.instances.map((i) => i.name)).toEqual(["alpha", "beta"]);
    expect(data.instances[0]).toMatchObject({
      name: "alpha",
      role: null,
      provider: null,
      region: null,
      health: null,
    });
  });

  it("tolerates a null/absent list", () => {
    expect(mapInstances({}).instances).toEqual([]);
    expect(mapInstances({ instances: null }).instances).toEqual([]);
  });
});

describe("mapTopology", () => {
  it("projects an embedded-object topology document (snake_case -> camelCase)", () => {
    const data = mapTopology({
      alpha: {
        instance: "alpha",
        topology: {
          hub: {
            instance: "alpha",
            dns_label: "alpha",
            remote_spokes: ["gamma", ""],
          },
          spokes: [{ name: "beta" }, { name: "" }],
        },
      },
    });

    expect(data.alpha).toEqual({
      instance: "alpha",
      hub: { instance: "alpha", dnsLabel: "alpha", remoteSpokes: ["gamma"] },
      spokes: [{ name: "beta" }],
    });
  });

  it("parses a topology document passed through as a JSON string", () => {
    const data = mapTopology({
      alpha: {
        topology: JSON.stringify({
          hub: { instance: "alpha", remote_spokes: [] },
          spokes: [{ name: "beta" }],
        }),
      },
    });

    expect(data.alpha?.hub?.instance).toBe("alpha");
    expect(data.alpha?.hub?.dnsLabel).toBeNull();
    expect(data.alpha?.spokes.map((s) => s.name)).toEqual(["beta"]);
  });

  it("degrades a malformed entry to an empty topology without throwing", () => {
    const data = mapTopology({
      alpha: { topology: "{not json" },
      beta: { topology: null },
      gamma: 42,
    });

    expect(data.alpha).toEqual({ instance: "alpha", hub: null, spokes: [] });
    expect(data.beta).toEqual({ instance: "beta", hub: null, spokes: [] });
    expect(data.gamma).toEqual({ instance: "gamma", hub: null, spokes: [] });
  });

  it("tolerates a null/non-record payload", () => {
    expect(mapTopology(null)).toEqual({});
    expect(mapTopology("nope")).toEqual({});
  });
});

describe("mapSdlc", () => {
  it("projects the full plane shape", () => {
    const data = mapSdlc({
      phases: [
        { key: "spec", sleSeconds: 100 },
        { key: "shipped", sleSeconds: 0 },
      ],
      cards: [
        {
          number: 10,
          title: "A",
          url: "https://x/10",
          phase: "spec",
          timeInPhaseSeconds: 50,
          scope: "agent/scope/int",
          rework: { count: 2, threshold: 3 },
          history: [
            { label: "agent/reviewable-spec", action: "labeled", at: "t" },
          ],
        },
      ],
      lane: [
        {
          number: 20,
          title: "B",
          url: "https://x/20",
          phase: "code",
          timeInPhaseSeconds: 5,
          escalation: "blocked",
        },
      ],
      matrix: {
        hubClusters: [
          {
            key: "env-a",
            cluster: "cluster-a",
            env: "prod",
            currentVersion: "v2",
          },
        ],
        rows: [
          {
            unreleased: true,
            issues: [{ number: 99, title: "u", url: "https://x/99" }],
          },
          {
            version: "v2",
            digest: "d2",
            date: "2026-09-01T00:00:00Z",
            hubClusters: ["env-a"],
            issues: [
              {
                number: 98,
                title: "s",
                url: "https://x/98",
                mergedAt: "2026-08-01T00:00:00Z",
                prs: [
                  {
                    number: 97,
                    title: "deliver s",
                    url: "https://x/pull/97",
                    author: "octo-dev",
                    mergedAt: "2026-08-01T00:00:00Z",
                  },
                ],
              },
            ],
            otherPRs: [
              {
                number: 540,
                title: "bump deps",
                url: "https://x/pull/540",
                author: "dependabot",
              },
            ],
          },
        ],
      },
    });

    expect(data.phases).toHaveLength(2);
    expect(data.cards[0]?.rework).toEqual({ count: 2, threshold: 3 });
    expect(data.cards[0]?.scope).toBe("agent/scope/int");
    expect(data.cards[0]?.history[0]?.label).toBe("agent/reviewable-spec");
    expect(data.lane[0]?.escalation).toBe("blocked");
    expect(data.matrix.hubClusters[0]?.currentVersion).toBe("v2");
    expect(data.matrix.rows[0]?.unreleased).toBe(true);
    // The Unreleased row decodes with no PRs groups.
    expect(data.matrix.rows[0]?.issues[0]?.prs).toEqual([]);
    expect(data.matrix.rows[0]?.otherPRs).toEqual([]);
    expect(data.matrix.rows[1]?.hubClusters).toEqual(["env-a"]);
    expect(data.matrix.rows[1]?.issues[0]?.mergedAt).toBe(
      "2026-08-01T00:00:00Z",
    );
    // A tracked issue carries its grouped PRs; one-off PRs decode into otherPRs.
    expect(data.matrix.rows[1]?.issues[0]?.prs[0]?.number).toBe(97);
    expect(data.matrix.rows[1]?.otherPRs[0]?.number).toBe(540);
    expect(data.matrix.rows[1]?.otherPRs[0]?.author).toBe("dependabot");
    expect(data.matrix.rows[1]?.otherPRs[0]?.mergedAt).toBeNull();
  });

  it("degrades a partial/empty payload to an empty board, lane, and matrix", () => {
    const data = mapSdlc({});
    expect(data.phases).toEqual([]);
    expect(data.cards).toEqual([]);
    expect(data.lane).toEqual([]);
    expect(data.matrix.hubClusters).toEqual([]);
    expect(data.matrix.rows).toEqual([]);
  });

  it("defaults missing rework/escalation to null and history to []", () => {
    const data = mapSdlc({
      cards: [{ number: 1, title: "t", url: "u", phase: "workable" }],
    });
    expect(data.cards[0]?.rework).toBeNull();
    expect(data.cards[0]?.escalation).toBeNull();
    expect(data.cards[0]?.history).toEqual([]);
  });
});

// Neutral placeholders only (hub-a/tenant-1/run-1); the fleet-dashboard source is
// public, so no fleet identifiers appear even in tests (firewall).
describe("mapE2eRuns", () => {
  it("maps the full recent-runs payload", () => {
    const data = mapE2eRuns({
      enabled: true,
      truncated: true,
      windowSeconds: 604800,
      limit: 20,
      runs: [
        {
          id: "tenant-1/ns/run-1",
          runId: "run-1",
          tenant: "tenant-1",
          cluster: "tenant-1",
          bodyCluster: "hub-a",
          namespace: "ns",
          revision: "abc1234",
          sourceRevision: "def5678",
          startedAt: "2026-01-01T00:00:00Z",
          endedAt: "2026-01-01T00:01:00Z",
          durationMs: 60000,
          verdict: "passed",
          reportingComplete: true,
          hasStructuredSteps: true,
          stepCount: 3,
        },
      ],
    });
    expect(data.enabled).toBe(true);
    expect(data.truncated).toBe(true);
    expect(data.windowSeconds).toBe(604800);
    const run = data.runs[0];
    if (!run) throw new Error("expected a run");
    // Trusted cluster is the tenant; bodyCluster is display metadata (E2E-LOG-08).
    expect(run.cluster).toBe("tenant-1");
    expect(run.bodyCluster).toBe("hub-a");
    expect(run.sourceRevision).toBe("def5678");
    expect(run.durationMs).toBe(60000);
  });

  it("handles the feature-off shape (enabled:false, no runs)", () => {
    const data = mapE2eRuns({ enabled: false });
    expect(data.enabled).toBe(false);
    expect(data.runs).toEqual([]);
  });

  it("defaults a malformed verdict to unknown and missing arrays to []", () => {
    const data = mapE2eRuns({
      enabled: true,
      runs: [{ id: "x", runId: "r", verdict: "weird" }],
    });
    expect(data.runs[0]?.verdict).toBe("unknown");
    expect(data.runs[0]?.bodyCluster).toBeNull();
    expect(data.runs[0]?.durationMs).toBeNull();
  });
});

describe("mapE2eRunDetail", () => {
  it("maps steps, logs, and planned steps; coerces an unknown step state", () => {
    const detail = mapE2eRunDetail({
      id: "tenant-1/ns/run-1",
      runId: "run-1",
      verdict: "failed",
      plannedSteps: [{ stepId: "a", name: "A" }],
      steps: [
        { stepId: "a", name: "A", state: "failed", durationMs: 10 },
        { stepId: "b", name: "B", state: "bogus" },
      ],
      logs: [{ sequence: 1, timestamp: "t", message: "hi", stepId: "a" }],
      logsTruncated: true,
    });
    expect(detail.verdict).toBe("failed");
    expect(detail.steps[0]?.state).toBe("failed");
    // An unrecognized state coerces to not-run, never passed.
    expect(detail.steps[1]?.state).toBe("not-run");
    expect(detail.logs[0]?.message).toBe("hi");
    expect(detail.logsTruncated).toBe(true);
  });

  it("degrades an empty/partial detail to empty collections", () => {
    const detail = mapE2eRunDetail({});
    expect(detail.steps).toEqual([]);
    expect(detail.logs).toEqual([]);
    expect(detail.plannedSteps).toEqual([]);
    expect(detail.verdict).toBe("unknown");
  });
});

describe("mapE2eStreamEvent", () => {
  it("maps a well-formed event", () => {
    const event = mapE2eStreamEvent({
      event: "step_finished",
      runId: "run-1",
      sequence: 4,
      timestamp: "t",
      stepId: "a",
      outcome: "passed",
      durationMs: 5,
    });
    expect(event).not.toBeNull();
    expect(event?.event).toBe("step_finished");
    expect(event?.outcome).toBe("passed");
    expect(event?.sequence).toBe(4);
  });

  it("returns null for an unknown event name so the frame is skipped", () => {
    expect(mapE2eStreamEvent({ event: "mystery", sequence: 1 })).toBeNull();
    expect(mapE2eStreamEvent(null)).toBeNull();
  });
});

describe("mapE2eRunLogPage", () => {
  it("maps entries, cursor, and hasMore (E2E-LOG-19)", () => {
    const page = mapE2eRunLogPage({
      entries: [
        { sequence: 1, timestamp: "t1", message: "a" },
        {
          sequence: 2,
          timestamp: "t2",
          message: "b",
          stepId: "s",
          truncated: true,
        },
      ],
      nextCursor: "42",
      hasMore: true,
    });
    expect(page.entries.map((l) => l.sequence)).toEqual([1, 2]);
    expect(page.entries[1]?.stepId).toBe("s");
    expect(page.entries[1]?.truncated).toBe(true);
    expect(page.nextCursor).toBe("42");
    expect(page.hasMore).toBe(true);
  });

  it("defaults a stale/empty page to no entries, no cursor, no more", () => {
    const page = mapE2eRunLogPage(undefined);
    expect(page.entries).toEqual([]);
    expect(page.nextCursor).toBeNull();
    expect(page.hasMore).toBe(false);
  });
});

describe("mapE2eStreamLog", () => {
  it("maps a live log frame into a reduced line (E2E-LOG-21)", () => {
    const line = mapE2eStreamLog({
      sequence: 7,
      timestamp: "t7",
      stepId: "s",
      message: "live",
      truncated: false,
    });
    expect(line).not.toBeNull();
    expect(line?.sequence).toBe(7);
    expect(line?.message).toBe("live");
  });

  it("returns null for a missing frame so the stream is not dropped", () => {
    expect(mapE2eStreamLog(null)).toBeNull();
    expect(mapE2eStreamLog(undefined)).toBeNull();
  });
});

describe("mapSdlcAgents", () => {
  // Verifies: SDLC-AGT-02.01
  it("projects the full plane shape (camelCase wire -> domain)", () => {
    const data = mapSdlcAgents({
      envs: ["integration", "production"],
      agents: [
        {
          name: "spec-review",
          env: "integration",
          replicasOn: 2,
          replicasTotal: 3,
          queueDepth: 5,
          itemsProcessed: 120,
          itemsFailed: 1,
          provisionSeconds: 45.5,
          series: {
            times: [100, 200],
            queueDepth: [4, 5],
            itemsProcessed: [118, 120],
            itemsFailed: [0, 1],
            provisionSeconds: [40, 45.5],
          },
        },
      ],
    });

    expect(data.envs).toEqual(["integration", "production"]);
    expect(data.agents).toHaveLength(1);
    expect(data.agents[0]).toEqual({
      name: "spec-review",
      env: "integration",
      replicasOn: 2,
      replicasTotal: 3,
      queueDepth: 5,
      itemsProcessed: 120,
      itemsFailed: 1,
      provisionSeconds: 45.5,
      series: {
        times: [100, 200],
        queueDepth: [4, 5],
        itemsProcessed: [118, 120],
        itemsFailed: [0, 1],
        provisionSeconds: [40, 45.5],
      },
    });
  });

  // Verifies: SDLC-AGT-02.02
  it('defaults missing fields to "" / 0 / [] rather than throwing', () => {
    const data = mapSdlcAgents({ agents: [{}] });

    expect(data.envs).toEqual([]);
    expect(data.agents).toEqual([
      {
        name: "",
        env: "",
        replicasOn: 0,
        replicasTotal: 0,
        queueDepth: 0,
        itemsProcessed: 0,
        itemsFailed: 0,
        provisionSeconds: 0,
        series: {
          times: [],
          queueDepth: [],
          itemsProcessed: [],
          itemsFailed: [],
          provisionSeconds: [],
        },
      },
    ]);
  });

  it("tolerates a null/non-record payload", () => {
    expect(mapSdlcAgents(null)).toEqual({ envs: [], agents: [] });
    expect(mapSdlcAgents(undefined)).toEqual({ envs: [], agents: [] });
  });
});
