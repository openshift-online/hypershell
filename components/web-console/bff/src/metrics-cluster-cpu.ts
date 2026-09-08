export const clusterCpuCapacityPromql =
  'sum(count by (instance) (node_cpu_seconds_total{mode="idle"}))';
export const clusterCpuUsedPromql =
  'sum(rate(node_cpu_seconds_total{mode!="idle"}[5m]))';

const usedExceedsCapacityToleranceCores = 0.01;

export interface ClusterCpuCores {
  available_cores: number;
  capacity_cores: number;
  used_cores: number;
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

    return value;
  } finally {
    clearTimeout(timeout);
  }
}

export async function queryClusterCpu(
  prometheusUrl: string,
  timeoutMs: number,
): Promise<ClusterCpuCores> {
  const [capacity_cores, used_cores] = await Promise.all([
    queryPrometheusInstant(prometheusUrl, clusterCpuCapacityPromql, timeoutMs),
    queryPrometheusInstant(prometheusUrl, clusterCpuUsedPromql, timeoutMs),
  ]);

  if (capacity_cores === 0) {
    throw new Error("No cluster CPU capacity data");
  }

  if (used_cores > capacity_cores + usedExceedsCapacityToleranceCores) {
    throw new Error("Inconsistent cluster CPU samples");
  }

  const available_cores = capacity_cores - used_cores;

  return {
    available_cores,
    capacity_cores,
    used_cores,
  };
}
