import {
  FanOutDomainProbePublisher,
  type DomainProbeSink,
  type ProbeDeliveryFailure,
} from "@openshift-online/hypershell-domain-probes/fan-out";

import type { TerminalProbe } from "../../application/terminal/terminal-probes";

const recentProbeLimit = 100;
const recentFailureLimit = 20;

interface PerformanceProbeTarget {
  clearMarks(name?: string): void;
  mark(name: string, options?: PerformanceMarkOptions): PerformanceMark;
}

export interface TerminalObservability {
  probes: FanOutDomainProbePublisher<TerminalProbe>;
  recentDeliveryFailures(): readonly Readonly<ProbeDeliveryFailure>[];
  recentProbes(): readonly TerminalProbe[];
}

function appendBounded<T>(target: T[], value: T, limit: number) {
  target.push(value);
  if (target.length > limit) {
    target.splice(0, target.length - limit);
  }
}

/**
 * Fans each terminal domain fact out to independent sinks (UIS-OBS-04): a
 * bounded product-health recorder and performance marks. A sink failure is
 * recorded in bounded delivery health and never changes a terminal result.
 * Probe payloads carry only bounded fields (WEB-TERM-13), so no sink needs
 * redaction.
 */
export function createTerminalObservability(
  options: {
    performanceTarget?: PerformanceProbeTarget;
  } = {},
): TerminalObservability {
  const recent: TerminalProbe[] = [];
  const failures: Readonly<ProbeDeliveryFailure>[] = [];
  const performanceTarget = options.performanceTarget ?? globalThis.performance;
  const recentSink: DomainProbeSink<TerminalProbe> = {
    id: "terminal-product-health",
    publish(value) {
      appendBounded(recent, value, recentProbeLimit);
    },
  };
  const performanceSink: DomainProbeSink<TerminalProbe> = {
    id: "terminal-performance",
    publish(value) {
      performanceTarget.clearMarks(value.name);
      performanceTarget.mark(value.name, { detail: value });
    },
  };
  const probes = new FanOutDomainProbePublisher<TerminalProbe>({
    failureReporter: {
      report(failure) {
        appendBounded(failures, failure, recentFailureLimit);
      },
    },
    sinks: [recentSink, performanceSink],
  });
  return {
    probes,
    recentDeliveryFailures: () => Object.freeze([...failures]),
    recentProbes: () => Object.freeze([...recent]),
  };
}
