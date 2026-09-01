import type {
  GatewayProfileProbe,
  GatewayProfileProbePublisher,
  GatewayWorkflowRuntime,
} from "@openshift-online/hypershell-gateway-management-ui";
import {
  FanOutDomainProbePublisher,
  type DomainProbeSink,
  type DomainProbeSinkSet,
  type ProbeDeliveryFailure,
  type ProbeDeliveryHealthSnapshot,
} from "@openshift-online/hypershell-domain-probes/fan-out";

const recentProbeLimit = 100;
const recentFailureLimit = 20;
const traceIdByteLength = 16;

/**
 * Creates a W3C trace identifier: a 16-byte random value rendered as 32
 * lowercase hex digits. A random 16-byte value is never the all-zero value
 * that the W3C Trace Context specification forbids.
 */
function createTraceId(): string {
  const bytes = new Uint8Array(traceIdByteLength);
  globalThis.crypto.getRandomValues(bytes);
  return Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0")).join(
    "",
  );
}

interface PerformanceProbeTarget {
  clearMarks(name?: string): void;
  mark(name: string, options?: PerformanceMarkOptions): PerformanceMark;
}

export interface GatewayProfileObservability {
  deliveryHealth(): Readonly<ProbeDeliveryHealthSnapshot>;
  probes: GatewayProfileProbePublisher;
  recentDeliveryFailures(): readonly Readonly<ProbeDeliveryFailure>[];
  recentProbes(): readonly GatewayProfileProbe[];
  // Records a delivery failure that surfaces asynchronously, outside the
  // synchronous probe fan-out, so a delivery the browser could not complete is
  // counted in delivery health instead of being lost.
  reportDeliveryFailure(failure: Readonly<ProbeDeliveryFailure>): void;
  runtime: GatewayWorkflowRuntime;
}

export interface GatewayProfileObservabilityOptions {
  additionalSinks?: readonly DomainProbeSink<GatewayProfileProbe>[];
  createCorrelationId?: () => string;
  createTraceId?: () => string;
  now?: () => string;
  performanceTarget?: PerformanceProbeTarget;
}

function appendBounded<T>(target: T[], value: T, limit: number) {
  target.push(value);
  if (target.length > limit) {
    target.splice(0, target.length - limit);
  }
}

export function createGatewayProfileObservability(
  options: GatewayProfileObservabilityOptions = {},
): GatewayProfileObservability {
  const failures: Readonly<ProbeDeliveryFailure>[] = [];
  const recent: GatewayProfileProbe[] = [];
  const performanceTarget = options.performanceTarget ?? globalThis.performance;
  const recentSink: DomainProbeSink<GatewayProfileProbe> = {
    id: "gateway-profile-product-health",
    publish(value) {
      appendBounded(recent, value, recentProbeLimit);
    },
  };
  const performanceSink: DomainProbeSink<GatewayProfileProbe> = {
    id: "gateway-profile-performance",
    publish(value) {
      performanceTarget.clearMarks(value.name);
      performanceTarget.mark(value.name, { detail: value });
    },
  };
  const additionalSinks = options.additionalSinks ?? [];
  const [firstAdditionalSink, ...remainingAdditionalSinks] = additionalSinks;
  const sinks: DomainProbeSinkSet<GatewayProfileProbe> =
    firstAdditionalSink === undefined
      ? [recentSink, performanceSink]
      : [
          recentSink,
          firstAdditionalSink,
          ...remainingAdditionalSinks,
          performanceSink,
        ];
  const publisher = new FanOutDomainProbePublisher<GatewayProfileProbe>({
    failureReporter: {
      report(failure) {
        appendBounded(failures, failure, recentFailureLimit);
      },
    },
    sinks,
  });

  return {
    deliveryHealth: () => publisher.healthSnapshot(),
    probes: publisher,
    recentDeliveryFailures: () => Object.freeze([...failures]),
    recentProbes: () => Object.freeze([...recent]),
    reportDeliveryFailure: (failure) => {
      publisher.reportDeliveryFailure(failure);
    },
    runtime: {
      createCorrelationId:
        options.createCorrelationId ?? (() => globalThis.crypto.randomUUID()),
      createTraceId: options.createTraceId ?? createTraceId,
      now: options.now ?? (() => new Date().toISOString()),
    },
  };
}
