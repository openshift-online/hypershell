import {
  FanOutDomainProbePublisher,
  type DomainProbeSink,
  type ProbeDeliveryFailure,
} from "@openshift-online/hypershell-domain-probes/fan-out";

import type { PlatformPolicyProbe } from "../../application/platform-policies/platform-policy-probes";

const recentProbeLimit = 100;
const recentFailureLimit = 20;

interface PerformanceProbeTarget {
  clearMarks(name?: string): void;
  mark(name: string, options?: PerformanceMarkOptions): PerformanceMark;
}

export interface PlatformPolicyObservability {
  probes: FanOutDomainProbePublisher<PlatformPolicyProbe>;
  recentDeliveryFailures(): readonly Readonly<ProbeDeliveryFailure>[];
  recentProbes(): readonly PlatformPolicyProbe[];
}

function appendBounded<T>(target: T[], value: T, limit: number) {
  target.push(value);
  if (target.length > limit) {
    target.splice(0, target.length - limit);
  }
}

/**
 * Fans each platform policy domain fact out to independent sinks (UIS-OBS-04):
 * a bounded product-health recorder and performance marks. A sink failure is
 * recorded in bounded delivery health and never changes the workflow result.
 */
export function createPlatformPolicyObservability(
  options: {
    performanceTarget?: PerformanceProbeTarget;
  } = {},
): PlatformPolicyObservability {
  const recent: PlatformPolicyProbe[] = [];
  const failures: Readonly<ProbeDeliveryFailure>[] = [];
  const performanceTarget = options.performanceTarget ?? globalThis.performance;
  const recentSink: DomainProbeSink<PlatformPolicyProbe> = {
    id: "platform-policy-product-health",
    publish(value) {
      appendBounded(recent, value, recentProbeLimit);
    },
  };
  const performanceSink: DomainProbeSink<PlatformPolicyProbe> = {
    id: "platform-policy-performance",
    publish(value) {
      performanceTarget.clearMarks(value.name);
      performanceTarget.mark(value.name, { detail: value });
    },
  };
  const probes = new FanOutDomainProbePublisher<PlatformPolicyProbe>({
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
