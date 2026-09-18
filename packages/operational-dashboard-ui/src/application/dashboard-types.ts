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

export type ProvisionSuccessCountWindow =
  "duration_24h" | "duration_lifetime" | "outcomes_24h";

export interface OperationalMetricProvisionOutcomes {
  failureCount24h: string;
  successCount24h: string;
  successCountWindow?: ProvisionSuccessCountWindow;
  successRatePercent: string;
}

export interface OperationalMetric {
  createdLast7Days?: string;
  createdLast30Days?: string;
  id: string;
  inventoryProviders?: Record<string, number>;
  inventoryRegions?: Record<string, number>;
  inventoryStatus?: Record<string, number>;
  podPhases?: OperationalMetricPodPhases;
  provisionDuration?: OperationalMetricProvisionDuration;
  provisionOutcomes?: OperationalMetricProvisionOutcomes;
  releaseDistribution?: Record<string, number>;
  status?: OperationalMetricStatus;
  hourlyTrend?: OperationalMetricTrend;
  successRateTrend?: OperationalMetricTrend;
  total?: string;
  trend?: OperationalMetricTrend;
  uniqueLoginsLast7Days?: string;
  uniqueLoginsLast30Days?: string;
  unit?: string;
  value: string;
}

export interface SignupTrendPoint {
  label: string;
  value: number;
}

export type DashboardMetricSourceId =
  | "cluster-cpu"
  | "cluster-memory"
  | "cluster-nodes"
  | "cluster-pods"
  | "gateway-metrics"
  | "gateway-release-distribution"
  | "platform-inventory"
  | "registered-users";

export interface OperationalDashboardMetrics {
  failedSources?: readonly DashboardMetricSourceId[];
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
