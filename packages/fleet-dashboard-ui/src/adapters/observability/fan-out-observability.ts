// Fan-out observability publisher: delivers each probe to every configured sink in
// order (domain-observability convention). It owns no buffering or retry - a sink
// owns its own transport - and never throws back into the caller, so one failing
// sink cannot break a workflow it is only observing.

import type {
  E2eStreamFact,
  E2eWorkflowProbe,
  ObservabilityPort,
  SdlcWorkflowProbe,
} from "../../application/observability";

export function createFanOutObservability(
  sinks: readonly ObservabilityPort[],
): ObservabilityPort {
  // deliver runs each sink inside its own try/catch so one failing sink can neither
  // break the workflow it only observes nor stop the remaining sinks. The "no raw
  // console" rule forbids logging here; a sink owns its own transport and failure
  // handling.
  const deliver = (fn: (sink: ObservabilityPort) => void): void => {
    for (const sink of sinks) {
      try {
        fn(sink);
      } catch {
        /* a sink is only observing; swallow and continue */
      }
    }
  };
  return {
    publishSdlcWorkflow(probe: SdlcWorkflowProbe): void {
      deliver((sink) => {
        sink.publishSdlcWorkflow(probe);
      });
    },
    publishE2eWorkflow(probe: E2eWorkflowProbe): void {
      deliver((sink) => {
        sink.publishE2eWorkflow(probe);
      });
    },
    publishE2eStream(fact: E2eStreamFact): void {
      deliver((sink) => {
        sink.publishE2eStream(fact);
      });
    },
  };
}
