import {
  FanOutDomainProbePublisher,
  type DomainProbeSink,
  type ProbeDeliveryFailure,
} from "@openshift-online/hypershell-domain-probes/fan-out";

import type { DocsProbe } from "../../application/docs/docs-probes";

const recentProbeLimit = 100;
const recentFailureLimit = 20;

interface PerformanceProbeTarget {
  clearMarks(name?: string): void;
  mark(name: string, options?: PerformanceMarkOptions): PerformanceMark;
}

export interface DocsObservability {
  probes: FanOutDomainProbePublisher<DocsProbe>;
  recentDeliveryFailures(): readonly Readonly<ProbeDeliveryFailure>[];
  recentProbes(): readonly DocsProbe[];
}

function appendBounded<T>(target: T[], value: T, limit: number) {
  target.push(value);
  if (target.length > limit) {
    target.splice(0, target.length - limit);
  }
}

/**
 * Fans each docs domain fact out to independent sinks (UIS-OBS-04): a bounded
 * product-health recorder and performance marks. A sink failure is recorded in
 * bounded delivery health and never changes what the page shows.
 */
export function createDocsObservability(
  options: { performanceTarget?: PerformanceProbeTarget } = {},
): DocsObservability {
  const recent: DocsProbe[] = [];
  const failures: Readonly<ProbeDeliveryFailure>[] = [];
  const performanceTarget = options.performanceTarget ?? globalThis.performance;
  const recentSink: DomainProbeSink<DocsProbe> = {
    id: "docs-product-health",
    publish(value) {
      appendBounded(recent, value, recentProbeLimit);
    },
  };
  const performanceSink: DomainProbeSink<DocsProbe> = {
    id: "docs-performance",
    publish(value) {
      performanceTarget.clearMarks(value.name);
      performanceTarget.mark(value.name, { detail: value });
    },
  };
  const probes = new FanOutDomainProbePublisher<DocsProbe>({
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
