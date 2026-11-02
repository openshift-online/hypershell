export interface OperationalMetricTrendPoint {
  label: string;
  value: number | null;
}

export interface OperationalMetricTrend {
  points: readonly OperationalMetricTrendPoint[];
}

export type DashboardTimeRange = 7 | 30;

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
  p99?: string;
  target?: string;
  p99Trend?: OperationalMetricTrend;
  p50Trend?: OperationalMetricTrend;
  p95Trend?: OperationalMetricTrend;
}

export interface OperationalMetricResourceReadiness {
  podReadinessPercent?: string;
  sandboxReadinessPercent?: string;
  unreadyResources?: string;
}

export interface OperationalMetricCertificateHealth {
  expiredCertificates: string;
  soonestDaysToExpiry?: string;
  statusUnavailable: string;
  totalCertificates: string;
  validCertificates: string;
}

export interface OperationalMetricOidcClientSync {
  accuracyPercent?: string;
  expectedClients?: string;
  keycloakStatus: "connected" | "disconnected" | "unknown";
  syncedClients?: string;
}

export interface OperationalMetricStateAccuracy {
  accuracyPercent?: string;
  observedMatchingDesired?: string;
  staleResources: string;
  totalResources?: string;
}

export type ProvisionSuccessCountWindow =
  "duration_24h" | "duration_lifetime" | "outcomes_24h";

export interface OperationalMetricProvisionOutcomes {
  failureCount24h: string;
  successCount24h: string;
  successCountWindow?: ProvisionSuccessCountWindow;
  successRatePercent: string;
  successRateTrend?: OperationalMetricTrend;
}

export interface OperationalMetric {
  createdLast7Days?: string;
  createdLast30Days?: string;
  createdTrend?: OperationalMetricTrend;
  dailyActiveUsers?: string;
  id: string;
  inactiveUsersLast30Days?: string;
  inventoryProviders?: Record<string, number>;
  inventoryRegions?: Record<string, number>;
  inventoryStatus?: Record<string, number>;
  orphanedSandboxes?: number;
  expiringSandboxes?: number;
  idleSandboxes?: number;
  podPhases?: OperationalMetricPodPhases;
  resourceReadiness?: OperationalMetricResourceReadiness;
  certificateHealth?: OperationalMetricCertificateHealth;
  oidcClientSync?: OperationalMetricOidcClientSync;
  stateAccuracy?: OperationalMetricStateAccuracy;
  provisionDuration?: OperationalMetricProvisionDuration;
  provisionOutcomes?: OperationalMetricProvisionOutcomes;
  reasonBreakdown?: Record<string, number>;
  releaseDistribution?: Record<string, number>;
  status?: OperationalMetricStatus;
  hourlyTrend?: OperationalMetricTrend;
  successRateTrend?: OperationalMetricTrend;
  total?: string;
  trend?: OperationalMetricTrend;
  usersWithGateway?: string;
  gatewayConversionPercent?: string;
  uniqueLoginsLast7Days?: string;
  uniqueLoginsLast30Days?: string;
  retention7DaysPercent?: string;
  unit?: string;
  value: string;
}

export interface SignupTrendPoint {
  label: string;
  value: number;
}

export type DashboardMetricSourceId =
  | "adoption"
  | "cluster-cpu"
  | "cluster-memory"
  | "cluster-nodes"
  | "cluster-pods"
  | "gateway-metrics"
  | "gateway-release-distribution"
  | "resource-readiness"
  | "certificate-health"
  | "platform-inventory"
  | "registered-users";

/** Metric source id used only on reliability-dashboard payloads (REL-DASH / ARM). */
export type ReliabilityMetricSourceId =
  "api-reliability" | "control-plane-reconciliation" | "oidc-client-sync";

export type ReliabilityMetricId =
  | "api-request-rate"
  | "api-error-rate"
  | "api-latency"
  | "api-p95-latency"
  | "oidc-client-sync"
  | "state-accuracy"
  | "reconciliation-failures"
  | "reconciliation-retries"
  | "reconciliation-successes"
  | "reconciliation-lag"
  | "stale-resource-status-count";

export type UsageMetricSourceId =
  "usage-gateways" | "usage-sandboxes" | "usage-sandbox-lifetime";

export type UsageMetricId =
  | "gateway-request-volume"
  | "gateway-error-rate"
  | "active-gateways"
  | "unused-gateways"
  | "sandbox-utilization"
  | "sandbox-lifetime";

export interface UsageMetricTrendPoint {
  label: string;
  value: number | null;
}

export interface UsageMetricTrend {
  coverageEnd?: string;
  coverageStart?: string;
  isPartial?: boolean;
  points: readonly UsageMetricTrendPoint[];
}

export interface UsageSandboxLifetime {
  mean?: string;
  p50?: string;
  p95?: string;
}

export interface UsageMetric {
  activeSharePercent?: string;
  id: UsageMetricId;
  sandboxLifetime?: UsageSandboxLifetime;
  total?: string;
  trend?: UsageMetricTrend;
  unit?: string;
  value: string;
}

export interface UsageDashboardMetrics {
  failedSources?: readonly UsageMetricSourceId[];
  lastSuccessfulRefresh: Date;
  metrics: readonly UsageMetric[];
  timeRange?: DashboardTimeRange;
}

export type DashboardFailedSourceId =
  DashboardMetricSourceId | ReliabilityMetricSourceId | UsageMetricSourceId;

export interface OperationalDashboardMetrics {
  failedSources?: readonly DashboardFailedSourceId[];
  lastSuccessfulRefresh: Date;
  metrics: readonly OperationalMetric[];
  timeRange?: DashboardTimeRange;
}

export interface DashboardInvocationContext {
  correlationId: string;
  isAdmin?: boolean;
  timeRange?: DashboardTimeRange;
  signal?: AbortSignal;
}

export interface DashboardOperationalMetricsOptions {
  isAdmin?: boolean;
}

/** Application-owned driven port for dashboard metrics. */
export interface DashboardControlPlane {
  getOperationalMetrics(
    context: DashboardInvocationContext,
  ): Promise<OperationalDashboardMetrics>;
  getReliabilityMetrics(
    context: DashboardInvocationContext,
  ): Promise<OperationalDashboardMetrics>;
  getUsageMetrics?(
    context: DashboardInvocationContext,
  ): Promise<UsageDashboardMetrics>;
  getAdoptionMetrics(
    context: DashboardInvocationContext,
  ): Promise<OperationalDashboardMetrics>;
}

/** Driving entry port used by the dashboard presentation adapters. */
export interface DashboardOperations {
  getOperationalMetrics(
    signal?: AbortSignal,
    timeRange?: DashboardTimeRange,
    options?: DashboardOperationalMetricsOptions,
  ): Promise<OperationalDashboardMetrics>;
  getReliabilityMetrics(
    signal?: AbortSignal,
  ): Promise<OperationalDashboardMetrics>;
  getUsageMetrics(
    signal?: AbortSignal,
    timeRange?: DashboardTimeRange,
    options?: DashboardOperationalMetricsOptions,
  ): Promise<UsageDashboardMetrics>;
  getAdoptionMetrics(
    signal?: AbortSignal,
    timeRange?: DashboardTimeRange,
    options?: DashboardOperationalMetricsOptions,
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
