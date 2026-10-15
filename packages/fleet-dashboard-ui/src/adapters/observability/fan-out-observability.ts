// Fan-out observability publisher: delivers each probe to every configured sink in
// order (domain-observability convention). It owns no buffering or retry - a sink
// owns its own transport - and never throws back into the caller, so one failing
// sink cannot break a workflow it is only observing.

import type {
  ObservabilityPort,
  SdlcWorkflowProbe,
} from "../../application/observability";

export function createFanOutObservability(
  sinks: readonly ObservabilityPort[],
): ObservabilityPort {
  return {
    publishSdlcWorkflow(probe: SdlcWorkflowProbe): void {
      for (const sink of sinks) {
        try {
          sink.publishSdlcWorkflow(probe);
        } catch {
          // A sink is only observing the workflow; a failing sink must never break
          // the workflow it watches, nor stop the remaining sinks. Swallow (the
          // "no raw console" rule forbids logging here; a sink owns its own
          // transport and failure handling).
        }
      }
    },
  };
}
