import type { DomainProbe, DomainProbePublisher } from "./domain-probe.js";

export interface DomainProbeSink<TProbe extends DomainProbe = DomainProbe> {
  readonly id: string;
  publish(probe: TProbe): void;
}

export type DomainProbeSinkSet<TProbe extends DomainProbe> = readonly [
  DomainProbeSink<TProbe>,
  DomainProbeSink<TProbe>,
  ...DomainProbeSink<TProbe>[],
];

export interface ProbeDeliveryFailure {
  readonly errorType: string;
  readonly probeName: string;
  readonly schemaVersion: number;
  readonly sinkId: string;
}

export interface ProbeDeliveryFailureReporter {
  report(failure: Readonly<ProbeDeliveryFailure>): void;
}

export interface ProbeDeliveryHealthSnapshot {
  readonly deliveryFailureCount: number;
  readonly diagnosticFailureCount: number;
  readonly lastFailure: Readonly<ProbeDeliveryFailure> | null;
}

export interface FanOutDomainProbePublisherOptions<TProbe extends DomainProbe> {
  readonly failureReporter: ProbeDeliveryFailureReporter;
  readonly sinks: DomainProbeSinkSet<TProbe>;
}

function deepFreeze<T>(value: T): Readonly<T> {
  if (typeof value !== "object" || value === null) {
    return value;
  }

  for (const child of Object.values(value)) {
    deepFreeze(child);
  }

  return Object.isFrozen(value) ? value : Object.freeze(value);
}

function errorType(error: unknown): string {
  return error instanceof Error ? error.name : typeof error;
}

/**
 * Delivers synchronously in configured order without buffering or retry.
 * Transport sinks must enqueue into their own bounded buffers and own flush policy.
 */
export class FanOutDomainProbePublisher<
  TProbe extends DomainProbe,
> implements DomainProbePublisher<TProbe> {
  readonly #failureReporter: ProbeDeliveryFailureReporter;
  readonly #sinks: DomainProbeSinkSet<TProbe>;
  #deliveryFailureCount = 0;
  #diagnosticFailureCount = 0;
  #lastFailure: Readonly<ProbeDeliveryFailure> | null = null;

  constructor(options: FanOutDomainProbePublisherOptions<TProbe>) {
    if (options.sinks.length < 2) {
      throw new TypeError("Domain probe fan-out requires at least two sinks");
    }

    if (options.sinks.some(({ id }) => id.trim() === "")) {
      throw new TypeError("Domain probe sink identifiers must not be blank");
    }

    const sinkIds = new Set(options.sinks.map(({ id }) => id));
    if (sinkIds.size !== options.sinks.length) {
      throw new TypeError("Domain probe sink identifiers must be unique");
    }

    this.#failureReporter = options.failureReporter;
    this.#sinks = Object.freeze([...options.sinks]);
  }

  publish(probe: TProbe): void {
    const immutableProbe = deepFreeze(probe) as TProbe;

    for (const sink of this.#sinks) {
      try {
        sink.publish(immutableProbe);
      } catch (error) {
        this.#recordFailure(
          Object.freeze({
            errorType: errorType(error),
            probeName: immutableProbe.name,
            schemaVersion: immutableProbe.schemaVersion,
            sinkId: sink.id,
          }),
        );
      }
    }
  }

  /**
   * Records a delivery failure discovered outside the synchronous publish path,
   * such as an asynchronous span export that a transport sink could not
   * complete after buffering. It feeds the same health accounting as an inline
   * sink failure, so an out-of-band loss is observable through
   * {@link healthSnapshot} rather than silently dropped.
   */
  reportDeliveryFailure(failure: Readonly<ProbeDeliveryFailure>): void {
    this.#recordFailure(Object.freeze({ ...failure }));
  }

  #recordFailure(failure: Readonly<ProbeDeliveryFailure>): void {
    this.#deliveryFailureCount += 1;
    this.#lastFailure = failure;

    try {
      this.#failureReporter.report(failure);
    } catch {
      this.#diagnosticFailureCount += 1;
    }
  }

  healthSnapshot(): Readonly<ProbeDeliveryHealthSnapshot> {
    return Object.freeze({
      deliveryFailureCount: this.#deliveryFailureCount,
      diagnosticFailureCount: this.#diagnosticFailureCount,
      lastFailure: this.#lastFailure,
    });
  }
}
