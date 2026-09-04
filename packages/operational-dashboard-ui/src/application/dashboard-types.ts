export interface OperationalMetricTrendPoint {
  label: string;
  value: number;
}

export interface OperationalMetricTrend {
  points: readonly OperationalMetricTrendPoint[];
}

export interface OperationalMetricStatus {
  degraded?: number;
  failed?: number;
  healthy?: number;
  provisioning?: number;
}

export interface OperationalMetricPodPhases {
  failed: number;
  pending: number;
  running: number;
  succeeded: number;
  unknown: number;
}

export interface OperationalMetricProvisionDuration {
  mean: string;
  p50: string;
  p95: string;
}

export interface OperationalMetric {
  id: string;
  podPhases?: OperationalMetricPodPhases;
  provisionDuration?: OperationalMetricProvisionDuration;
  status?: OperationalMetricStatus;
  total?: string;
  trend?: OperationalMetricTrend;
  unit?: string;
  value: string;
}

export interface SignupTrendPoint {
  label: string;
  value: number;
}

export interface OperationalDashboardMetrics {
  lastSuccessfulRefresh: Date;
  metrics: readonly OperationalMetric[];
}

export interface DashboardInvocationContext {
  correlationId: string;
  signal?: AbortSignal;
}

/** Application-owned driven port for operational dashboard metrics. */
export interface DashboardControlPlane {
  getOperationalMetrics(
    context: DashboardInvocationContext,
  ): Promise<OperationalDashboardMetrics>;
}

/** Driving entry port used by the operational dashboard presentation adapters. */
export interface DashboardOperations {
  getOperationalMetrics(
    signal?: AbortSignal,
  ): Promise<OperationalDashboardMetrics>;
}

/** Application-owned port for nondeterministic workflow context. */
export interface DashboardWorkflowRuntime {
  createCorrelationId(): string;
}

export type {
  DashboardProbe,
  DashboardProbeAction,
  DashboardProbeName,
  DashboardProbeOutcome,
  DashboardProbePublisher,
  DashboardWorkflowAction,
} from "./dashboard-probes";
