import type {
  GatewayProbe,
  GatewayProbePublisher,
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

export interface GatewayObservability {
  deliveryHealth(): Readonly<ProbeDeliveryHealthSnapshot>;
  probes: GatewayProbePublisher;
  recentDeliveryFailures(): readonly Readonly<ProbeDeliveryFailure>[];
  recentProbes(): readonly GatewayProbe[];
  runtime: GatewayWorkflowRuntime;
}

export interface GatewayObservabilityOptions {
  additionalSinks?: readonly DomainProbeSink<GatewayProbe>[];
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

export function createGatewayObservability(
  options: GatewayObservabilityOptions = {},
): GatewayObservability {
  const failures: Readonly<ProbeDeliveryFailure>[] = [];
  const recent: GatewayProbe[] = [];
  const performanceTarget = options.performanceTarget ?? globalThis.performance;
  const recentSink: DomainProbeSink<GatewayProbe> = {
    id: "gateway-product-health",
    publish(value) {
      appendBounded(recent, value, recentProbeLimit);
    },
  };
  const performanceSink: DomainProbeSink<GatewayProbe> = {
    id: "gateway-performance",
    publish(value) {
      performanceTarget.clearMarks(value.name);
      performanceTarget.mark(value.name, { detail: value });
    },
  };
  const additionalSinks = options.additionalSinks ?? [];
  const [firstAdditionalSink, ...remainingAdditionalSinks] = additionalSinks;
  const sinks: DomainProbeSinkSet<GatewayProbe> =
    firstAdditionalSink === undefined
      ? [recentSink, performanceSink]
      : [
          recentSink,
          firstAdditionalSink,
          ...remainingAdditionalSinks,
          performanceSink,
        ];
  const publisher = new FanOutDomainProbePublisher<GatewayProbe>({
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
    runtime: {
      createCorrelationId:
        options.createCorrelationId ?? (() => globalThis.crypto.randomUUID()),
      createTraceId: options.createTraceId ?? createTraceId,
      now: options.now ?? (() => new Date().toISOString()),
    },
  };
}
