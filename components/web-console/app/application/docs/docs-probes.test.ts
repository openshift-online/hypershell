import { describe, expect, it } from "vitest";

import { createDocsFacts, type DocsProbe } from "./docs-probes";

function recordingFacts() {
  const published: DocsProbe[] = [];
  const facts = createDocsFacts({
    probes: {
      publish(probe) {
        published.push(probe);
      },
    },
    runtime: {
      createCorrelationId: () => "11111111-1111-4111-8111-111111111111",
      createTraceId: () => "0af7651916cd43dd8448eb211c80319c",
      now: () => "2026-11-24T00:00:00.000Z",
    },
  });
  return { facts, published };
}

describe("docs facts", () => {
  // Verifies: WEB-UDOC-12, WEB-UDOC-12.01
  it("publishes the route template without reader context", () => {
    const { facts, published } = recordingFacts();
    facts.pageViewed("/docs/refusals/resource-limits", "signed-out");

    expect(published).toHaveLength(1);
    expect(published[0]).toMatchObject({
      fields: {
        route: "/docs/refusals/resource-limits",
        session: "signed-out",
      },
      name: "docs.page.viewed",
      schemaVersion: 1,
    });
    const serialized = JSON.stringify(published[0]);
    expect(serialized).not.toContain("sandbox-cpu-limit");
    expect(serialized).not.toContain("team-a");
    expect(Object.isFrozen(published[0])).toBe(true);
  });

  // Verifies: WEB-UDOC-12
  it("publishes sign-in prompts and runbook runs with bounded fields", () => {
    const { facts, published } = recordingFacts();
    facts.signInPromptShown("/docs/runbooks/github-jira");
    facts.runbookStepRun("started", {
      action: "create",
      runbook: "github-jira",
    });
    facts.runbookStepRun("succeeded", {
      action: "create",
      runbook: "github-jira",
    });
    facts.runbookStepRun("failed", {
      action: "check",
      failureKind: "denied",
      runbook: "gitlab-jira",
    });

    expect(published.map((probe) => probe.name)).toEqual([
      "docs.sign-in-prompt.shown",
      "docs.runbook-step.run.started",
      "docs.runbook-step.run.succeeded",
      "docs.runbook-step.run.failed",
    ]);
    expect(published[1]?.fields).toEqual({
      action: "create",
      failureKind: null,
      runbook: "github-jira",
      step: 1,
    });
    expect(published[3]?.fields).toEqual({
      action: "check",
      failureKind: "denied",
      runbook: "gitlab-jira",
      step: 1,
    });
  });
});
