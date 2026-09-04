export const clusterPodsCapacityPromql =
  'sum(kube_node_status_allocatable{resource="pods"})';
export const clusterPodsUsedPromql = "count(kube_pod_info)";

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

export interface ClusterPodsCounts {
  available_pods: number;
  capacity_pods: number;
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
  prometheusUrl: string,
  query: string,
  timeoutMs: number,
): Promise<number> {
  const queryUrl = new URL("/api/v1/query", prometheusUrl);
  queryUrl.searchParams.set("query", query);

  const controller = new AbortController();
  const timeoutReason = new Error("Prometheus query timed out");
  const timeout = setTimeout(() => {
    controller.abort(timeoutReason);
  }, timeoutMs);

  try {
    const response = await fetch(queryUrl, { signal: controller.signal });
    if (!response.ok) {
      throw new Error("Prometheus query request failed");
    }

    const body = (await response.json()) as PrometheusQueryResponse;
    if (body.status !== "success") {
      throw new Error("Prometheus query returned non-success status");
    }

    const samples = body.data?.result ?? [];
    if (samples.length === 0) {
      return 0;
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
  prometheusUrl: string,
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

  return {
    available_pods,
    capacity_pods,
    phase_failed_pods,
    phase_pending_pods,
    phase_running_pods,
    phase_succeeded_pods,
    phase_unknown_pods,
    used_pods,
  };
}
