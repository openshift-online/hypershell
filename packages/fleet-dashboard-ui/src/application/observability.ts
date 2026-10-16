// Domain observability port (agent-sdlc-visibility-dashboard.spec SDLC-VIS-15 /
// specs/standards/ui/domain-observability.spec.md). Production browser/BFF code
// publishes typed workflow facts through this fan-out port instead of making raw
// console or direct telemetry calls. The port is deliberately framework- and
// transport-free; concrete sinks (and their timestamping) live in the adapter
// layer, so application code never touches Date/console (the package lint bans both
// here).

/** Lifecycle outcomes a workflow probe reports. */
export type SdlcWorkflowOutcome =
  "started" | "succeeded" | "failed" | "cancelled";

/** A typed fact about an Agent SDLC workflow (loading its data). */
export interface SdlcWorkflowProbe {
  readonly name: "sdlc.load";
  readonly outcome: SdlcWorkflowOutcome;
}

/** The e2e-run workflows that publish a lifecycle outcome (E2E-LOG-13). */
export type E2eWorkflowName = "e2e.runs.load" | "e2e.run.load";

/** Lifecycle outcomes an e2e workflow reports (same set as the SDLC workflow). */
export type E2eWorkflowOutcome = SdlcWorkflowOutcome;

/** A typed fact about an e2e-run workflow (loading recent runs or one run). */
export interface E2eWorkflowProbe {
  readonly name: E2eWorkflowName;
  readonly outcome: E2eWorkflowOutcome;
}

/** Stream-lifecycle dependency facts for the live e2e tail (E2E-LOG-13). */
export type E2eStreamFactKind =
  "connect" | "disconnect" | "reconnect" | "dropped-entry";

/** A typed fact about the live e2e stream dependency. */
export interface E2eStreamFact {
  readonly name: "e2e.stream";
  readonly kind: E2eStreamFactKind;
  /** Entries lost on a dropped-entry fact; 0 for the lifecycle facts. */
  readonly droppedEntries: number;
}

/** The fan-out observability port the dashboard publishes domain facts through. */
export interface ObservabilityPort {
  publishSdlcWorkflow(probe: SdlcWorkflowProbe): void;
  publishE2eWorkflow(probe: E2eWorkflowProbe): void;
  publishE2eStream(fact: E2eStreamFact): void;
}

/** Default port: publishes nowhere. Used when no sinks are wired (and in tests). */
export const noopObservability: ObservabilityPort = {
  publishSdlcWorkflow() {
    /* no sink configured */
  },
  publishE2eWorkflow() {
    /* no sink configured */
  },
  publishE2eStream() {
    /* no sink configured */
  },
};

/** Build a workflow probe for an SDLC-load outcome. */
export function sdlcWorkflow(outcome: SdlcWorkflowOutcome): SdlcWorkflowProbe {
  return { name: "sdlc.load", outcome };
}

/** Build an e2e workflow probe for a named workflow and its outcome. */
export function e2eWorkflow(
  name: E2eWorkflowName,
  outcome: E2eWorkflowOutcome,
): E2eWorkflowProbe {
  return { name, outcome };
}

/** Build an e2e stream dependency fact. */
export function e2eStreamFact(
  kind: E2eStreamFactKind,
  droppedEntries = 0,
): E2eStreamFact {
  return { name: "e2e.stream", kind, droppedEntries };
}
