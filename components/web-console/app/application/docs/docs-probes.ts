import type {
  DomainProbe,
  DomainProbePublisher,
} from "@openshift-online/hypershell-domain-probes";

import type { RunbookId } from "./runbook-placement";

/*
 * Closed, versioned domain facts for the docs pages (WEB-UDOC-12, UIS-OBS).
 * Fields are route templates and bounded enums only: no query value, gateway
 * or policy name, command, or command output ever enters a probe payload.
 */

/** Route templates of the docs pages (WEB-UDOC-01). */
export type DocsRouteTemplate =
  | "/docs"
  | "/docs/cli/install-hsctl"
  | "/docs/refusals/resource-limits"
  | "/docs/runbooks/github-jira"
  | "/docs/runbooks/gitlab-jira";

export type DocsSessionState = "signed-in" | "signed-out";

/** The runbook step a run belongs to. Only Step 1 runs in the browser. */
export type DocsRunbookStep = 1;

export type DocsRunbookStepAction = "check" | "create" | "recheck";

export type DocsRunFailureKind =
  "cancelled" | "conflict" | "denied" | "not-found" | "unavailable" | "unknown";

type Context = DomainProbe["context"];

export type DocsPageViewedProbe = DomainProbe<
  "docs.page.viewed",
  1,
  {
    readonly route: DocsRouteTemplate;
    readonly session: DocsSessionState;
  }
>;

export type DocsSignInPromptShownProbe = DomainProbe<
  "docs.sign-in-prompt.shown",
  1,
  { readonly route: DocsRouteTemplate }
>;

export type DocsRunbookStepRunProbe = DomainProbe<
  | "docs.runbook-step.run.started"
  | "docs.runbook-step.run.succeeded"
  | "docs.runbook-step.run.failed",
  1,
  {
    readonly action: DocsRunbookStepAction;
    readonly failureKind: DocsRunFailureKind | null;
    readonly runbook: RunbookId;
    readonly step: DocsRunbookStep;
  }
>;

export type DocsProbe =
  DocsPageViewedProbe | DocsRunbookStepRunProbe | DocsSignInPromptShownProbe;

export type DocsProbePublisher = DomainProbePublisher<DocsProbe>;

/** Clock and identifiers come from the runtime port (UIS-HEX). */
export interface DocsProbeRuntime {
  createCorrelationId(): string;
  createTraceId(): string;
  now(): string;
}

function freezeProbe<TProbe extends DocsProbe>(
  name: TProbe["name"],
  context: Context,
  occurredAt: string,
  fields: TProbe["fields"],
): TProbe {
  return Object.freeze({
    context: Object.freeze({ ...context }),
    fields: Object.freeze({ ...fields }),
    name,
    occurredAt,
    schemaVersion: 1,
  }) as TProbe;
}

export interface DocsFacts {
  pageViewed(route: DocsRouteTemplate, session: DocsSessionState): void;
  signInPromptShown(route: DocsRouteTemplate): void;
  runbookStepRun(
    outcome: "failed" | "started" | "succeeded",
    fields: {
      readonly action: DocsRunbookStepAction;
      readonly failureKind?: DocsRunFailureKind;
      readonly runbook: RunbookId;
    },
  ): void;
}

/** Publishes the docs domain facts through the fan-out probe port. */
export function createDocsFacts({
  probes,
  runtime,
}: {
  probes: DocsProbePublisher;
  runtime: DocsProbeRuntime;
}): DocsFacts {
  const context = (): Context => ({
    correlationId: runtime.createCorrelationId(),
    traceId: runtime.createTraceId(),
  });
  return {
    pageViewed(route, session) {
      probes.publish(
        freezeProbe<DocsPageViewedProbe>(
          "docs.page.viewed",
          context(),
          runtime.now(),
          { route, session },
        ),
      );
    },
    signInPromptShown(route) {
      probes.publish(
        freezeProbe<DocsSignInPromptShownProbe>(
          "docs.sign-in-prompt.shown",
          context(),
          runtime.now(),
          { route },
        ),
      );
    },
    runbookStepRun(outcome, fields) {
      probes.publish(
        freezeProbe<DocsRunbookStepRunProbe>(
          `docs.runbook-step.run.${outcome}`,
          context(),
          runtime.now(),
          {
            action: fields.action,
            failureKind:
              outcome === "failed" ? (fields.failureKind ?? "unknown") : null,
            runbook: fields.runbook,
            step: 1,
          },
        ),
      );
    },
  };
}

const consumers = ["performance", "product-health", "trace"] as const;

export const docsProbeCatalog = Object.freeze([
  {
    allowedConsumers: consumers,
    deliveryClass: "best-effort",
    fields: {
      route: "route template; never a query value",
      session: "signed-in or signed-out",
    },
    name: "docs.page.viewed",
    owner: "docs",
    schemaVersion: 1,
    trigger: "A docs page renders for a reader",
  },
  {
    allowedConsumers: consumers,
    deliveryClass: "best-effort",
    fields: { route: "route template; never a query value" },
    name: "docs.sign-in-prompt.shown",
    owner: "docs",
    schemaVersion: 1,
    trigger: "A signed-out docs page shows its sign-in prompt",
  },
  ...(
    [
      ["docs.runbook-step.run.started", "A reader activates a runbook Run"],
      ["docs.runbook-step.run.succeeded", "A runbook run completes"],
      ["docs.runbook-step.run.failed", "A runbook run fails"],
    ] as const
  ).map(([name, trigger]) => ({
    allowedConsumers: consumers,
    deliveryClass: "best-effort",
    fields: {
      action: "bounded run enum",
      failureKind: "bounded failure enum; no raw error content",
      runbook: "runbook identifier enum",
      step: "step number",
    },
    name,
    owner: "docs",
    schemaVersion: 1,
    trigger,
  })),
]);
