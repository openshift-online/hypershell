import {
  FanOutDomainProbePublisher,
  type DomainProbeSink,
  type ProbeDeliveryFailure,
  type ProbeDeliveryFailureReporter,
} from "../src/fan-out.js";
import type { DomainProbe } from "../src/index.js";

type TestProbe =
  | DomainProbe<"fleet.list.started", 1, Readonly<{ outcome: "started" }>>
  | DomainProbe<
      "fleet.list.succeeded",
      1,
      Readonly<{ outcome: "succeeded"; resultCount: number }>
    >;

const startedProbe = (): TestProbe => ({
  context: {
    correlationId: "correlation-1",
    traceId: "trace-1",
  },
  fields: {
    outcome: "started",
  },
  name: "fleet.list.started",
  occurredAt: "2026-08-05T12:00:00.000Z",
  schemaVersion: 1,
});

function recordingSink(
  id: string,
  target: TestProbe[],
): DomainProbeSink<TestProbe> {
  return {
    id,
    publish(probe) {
      target.push(probe);
    },
  };
}

function recordingReporter(
  target: Readonly<ProbeDeliveryFailure>[],
): ProbeDeliveryFailureReporter {
  return {
    report(failure) {
      target.push(failure);
    },
  };
}

describe("FanOutDomainProbePublisher", () => {
  it("delivers the same immutable fact to every sink in configuration order", () => {
    const calls: string[] = [];
    const receivedByFirst: TestProbe[] = [];
    const receivedBySecond: TestProbe[] = [];
    const publisher = new FanOutDomainProbePublisher<TestProbe>({
      failureReporter: recordingReporter([]),
      sinks: [
        {
          id: "structured-log",
          publish(probe) {
            calls.push("structured-log");
            receivedByFirst.push(probe);
          },
        },
        {
          id: "metrics",
          publish(probe) {
            calls.push("metrics");
            receivedBySecond.push(probe);
          },
        },
      ],
    });
    const probe = Object.freeze(startedProbe());

    publisher.publish(probe);

    expect(calls).toEqual(["structured-log", "metrics"]);
    expect(receivedByFirst).toEqual([probe]);
    expect(receivedBySecond).toEqual([probe]);
    expect(receivedByFirst[0]).toBe(receivedBySecond[0]);
    expect(Object.isFrozen(probe)).toBe(true);
    expect(Object.isFrozen(probe.context)).toBe(true);
    expect(Object.isFrozen(probe.fields)).toBe(true);
    expect(publisher.healthSnapshot()).toEqual({
      deliveryFailureCount: 0,
      diagnosticFailureCount: 0,
      lastFailure: null,
    });
  });

  it("isolates a failing sink and reports only bounded diagnostic fields", () => {
    const received: TestProbe[] = [];
    const failures: Readonly<ProbeDeliveryFailure>[] = [];
    const publisher = new FanOutDomainProbePublisher<TestProbe>({
      failureReporter: recordingReporter(failures),
      sinks: [
        {
          id: "broken-exporter",
          publish() {
            throw new TypeError("secret payload must not escape");
          },
        },
        recordingSink("healthy-exporter", received),
      ],
    });
    const probe = startedProbe();

    expect(() => {
      publisher.publish(probe);
    }).not.toThrow();

    expect(received).toEqual([probe]);
    expect(failures).toEqual([
      {
        errorType: "TypeError",
        probeName: "fleet.list.started",
        schemaVersion: 1,
        sinkId: "broken-exporter",
      },
    ]);
    expect(publisher.healthSnapshot()).toEqual({
      deliveryFailureCount: 1,
      diagnosticFailureCount: 0,
      lastFailure: failures[0],
    });
    expect(JSON.stringify(failures)).not.toContain("secret payload");
  });

  it("keeps delivery non-throwing when the diagnostic reporter fails", () => {
    const received: TestProbe[] = [];
    const publisher = new FanOutDomainProbePublisher<TestProbe>({
      failureReporter: {
        report() {
          throw new Error("diagnostic unavailable");
        },
      },
      sinks: [
        {
          id: "broken-exporter",
          publish() {
            throw new Error("exporter unavailable");
          },
        },
        recordingSink("healthy-exporter", received),
      ],
    });

    expect(() => {
      publisher.publish(startedProbe());
    }).not.toThrow();
    expect(received).toHaveLength(1);
    expect(publisher.healthSnapshot()).toMatchObject({
      deliveryFailureCount: 1,
      diagnosticFailureCount: 1,
      lastFailure: { errorType: "Error" },
    });
  });

  it("protects later consumers from a sink that attempts mutation", () => {
    const received: TestProbe[] = [];
    const failures: Readonly<ProbeDeliveryFailure>[] = [];
    const publisher = new FanOutDomainProbePublisher<TestProbe>({
      failureReporter: recordingReporter(failures),
      sinks: [
        {
          id: "mutating-exporter",
          publish(probe) {
            (probe.fields as { outcome: string }).outcome = "tampered";
          },
        },
        recordingSink("healthy-exporter", received),
      ],
    });

    publisher.publish(startedProbe());

    expect(failures).toHaveLength(1);
    expect(received[0]?.fields.outcome).toBe("started");
  });

  it("rejects invalid sink sets", () => {
    const sink = recordingSink("structured-log", []);
    const reporter = recordingReporter([]);

    expect(
      () =>
        new FanOutDomainProbePublisher<TestProbe>({
          failureReporter: reporter,
          sinks: [sink] as never,
        }),
    ).toThrow("at least two sinks");
    expect(
      () =>
        new FanOutDomainProbePublisher<TestProbe>({
          failureReporter: reporter,
          sinks: [sink, recordingSink(" ", [])],
        }),
    ).toThrow("must not be blank");
    expect(
      () =>
        new FanOutDomainProbePublisher<TestProbe>({
          failureReporter: reporter,
          sinks: [sink, sink],
        }),
    ).toThrow("identifiers must be unique");
  });
});
