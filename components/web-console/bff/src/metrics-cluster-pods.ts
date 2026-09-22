import { fetchMetrics, type MetricsSource } from "./metrics-source.js";
import {
  alignDailyIntegerSeries,
  dailyRangeStepSeconds,
  queryPrometheusRangeScalarSamples,
  sevenDayUtcCalendarRange,
} from "./prometheus-range-query.js";

export const clusterPodsCapacityPromql =
  'sum(kube_node_status_allocatable{resource="pods"})';
export const clusterPodsUsedPromql = "count(kube_pod_info)";
export const clusterPodsDailyUsedPromql = clusterPodsUsedPromql;
export const clusterPodsDailyUsedStepSeconds = dailyRangeStepSeconds;

export type ClusterPodPhase =
  "Failed" | "Pending" | "Running" | "Succeeded" | "Unknown";

export const CLUSTER_POD_PHASES = [
  "Pending",
  "Running",
  "Succeeded",
  "Failed",
  "Unknown",
] as const satisfies readonly ClusterPodPhase[];

export function clusterPodPhasePromql(phase: ClusterPodPhase): string {
  return `sum(kube_pod_status_phase{phase="${phase}"})`;
}

export interface ClusterPodsDailyUsed {
  date: string;
  value: number;
}

export interface ClusterPodsCounts {
  available_pods: number;
  capacity_pods: number;
  daily_used?: ClusterPodsDailyUsed[];
  phase_failed_pods: number;
  phase_pending_pods: number;
  phase_running_pods: number;
  phase_succeeded_pods: number;
  phase_unknown_pods: number;
  used_pods: number;
}

interface PrometheusQueryResponse {
  status: string;
  data?: {
    result: {
      value: [string, string];
    }[];
  };
}

async function queryPrometheusInstant(
  prometheusUrl: MetricsSource,
  query: string,
  timeoutMs: number,
): Promise<number> {
  const controller = new AbortController();
  const timeoutReason = new Error("Prometheus query timed out");
  const timeout = setTimeout(() => {
    controller.abort(timeoutReason);
  }, timeoutMs);

  try {
    const response = await fetchMetrics(
      prometheusUrl,
      query,
      controller.signal,
    );
    if (!response.ok) {
      throw new Error("Prometheus query request failed");
    }

    const body = (await response.json()) as PrometheusQueryResponse;
    if (body.status !== "success") {
      throw new Error("Prometheus query returned non-success status");
    }

    const samples = body.data?.result ?? [];
    if (samples.length === 0) {
      throw new Error("Prometheus query returned no samples");
    }

    const sample = samples[0];
    const rawValue = sample?.value[1];
    if (rawValue === undefined) {
      throw new Error("Prometheus query returned invalid sample");
    }

    const value = Number(rawValue);
    if (!Number.isFinite(value) || value < 0) {
      throw new Error("Prometheus query returned invalid sample");
    }

    return Math.round(value);
  } finally {
    clearTimeout(timeout);
  }
}

export async function queryClusterPods(
  prometheusUrl: MetricsSource,
  timeoutMs: number,
): Promise<ClusterPodsCounts> {
  const [
    capacity_pods,
    used_pods,
    phase_pending_pods,
    phase_running_pods,
    phase_succeeded_pods,
    phase_failed_pods,
    phase_unknown_pods,
  ] = await Promise.all([
    queryPrometheusInstant(prometheusUrl, clusterPodsCapacityPromql, timeoutMs),
    queryPrometheusInstant(prometheusUrl, clusterPodsUsedPromql, timeoutMs),
    queryPrometheusInstant(
      prometheusUrl,
      clusterPodPhasePromql("Pending"),
      timeoutMs,
    ),
    queryPrometheusInstant(
      prometheusUrl,
      clusterPodPhasePromql("Running"),
      timeoutMs,
    ),
    queryPrometheusInstant(
      prometheusUrl,
      clusterPodPhasePromql("Succeeded"),
      timeoutMs,
    ),
    queryPrometheusInstant(
      prometheusUrl,
      clusterPodPhasePromql("Failed"),
      timeoutMs,
    ),
    queryPrometheusInstant(
      prometheusUrl,
      clusterPodPhasePromql("Unknown"),
      timeoutMs,
    ),
  ]);

  if (capacity_pods === 0) {
    throw new Error("No cluster pod capacity data");
  }

  if (used_pods > capacity_pods) {
    throw new Error("Inconsistent cluster pod samples");
  }

  const phaseTotal =
    phase_pending_pods +
    phase_running_pods +
    phase_succeeded_pods +
    phase_failed_pods +
    phase_unknown_pods;

  if (phaseTotal !== used_pods) {
    throw new Error("Inconsistent cluster pod phase samples");
  }

  const available_pods = capacity_pods - used_pods;

  const response: ClusterPodsCounts = {
    available_pods,
    capacity_pods,
    phase_failed_pods,
    phase_pending_pods,
    phase_running_pods,
    phase_succeeded_pods,
    phase_unknown_pods,
    used_pods,
  };

  try {
    const { end, start } = sevenDayUtcCalendarRange();
    const samples = await queryPrometheusRangeScalarSamples(
      prometheusUrl,
      clusterPodsDailyUsedPromql,
      start,
      end,
      `${String(clusterPodsDailyUsedStepSeconds)}s`,
      timeoutMs,
    );
    response.daily_used = alignDailyIntegerSeries(samples).map(
      ({ date, value }) => ({
        date,
        value,
      }),
    );
  } catch {
    // Omit trend only.
  }

  return response;
}
