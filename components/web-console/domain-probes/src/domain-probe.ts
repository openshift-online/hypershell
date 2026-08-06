export type ProbePrimitive = boolean | number | string | null;

export type ProbeValue =
  | ProbePrimitive
  | readonly ProbeValue[]
  | Readonly<{ [key: string]: ProbeValue }>;

export type ProbeFields = Readonly<Record<string, ProbeValue>>;

export interface DomainProbeContext {
  readonly correlationId: string;
  readonly operationId?: string;
  readonly parentInvocationId?: string;
  readonly retryAttempt?: number;
  readonly traceId?: string;
}

export interface DomainProbe<
  TName extends string = string,
  TVersion extends number = number,
  TFields extends ProbeFields = ProbeFields,
> {
  readonly context: Readonly<DomainProbeContext>;
  readonly fields: TFields;
  readonly name: TName;
  readonly occurredAt: string;
  readonly schemaVersion: TVersion;
}

export interface DomainProbePublisher<
  TProbe extends DomainProbe = DomainProbe,
> {
  publish(probe: TProbe): void;
}
