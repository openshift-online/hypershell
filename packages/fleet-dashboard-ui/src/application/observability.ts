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

/** The fan-out observability port the dashboard publishes domain facts through. */
export interface ObservabilityPort {
  publishSdlcWorkflow(probe: SdlcWorkflowProbe): void;
}

/** Default port: publishes nowhere. Used when no sinks are wired (and in tests). */
export const noopObservability: ObservabilityPort = {
  publishSdlcWorkflow() {
    /* no sink configured */
  },
};

/** Build a workflow probe for an SDLC-load outcome. */
export function sdlcWorkflow(outcome: SdlcWorkflowOutcome): SdlcWorkflowProbe {
  return { name: "sdlc.load", outcome };
}
