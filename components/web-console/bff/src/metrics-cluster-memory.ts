export const clusterMemoryCapacityPromql = "sum(node_memory_MemTotal_bytes)";
export const clusterMemoryAvailablePromql =
  "sum(node_memory_MemAvailable_bytes)";

export interface ClusterMemoryBytes {
  available_bytes: number;
  capacity_bytes: number;
  used_bytes: number;
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

export async function queryClusterMemory(
  prometheusUrl: string,
  timeoutMs: number,
): Promise<ClusterMemoryBytes> {
  const [capacity_bytes, available_bytes] = await Promise.all([
    queryPrometheusInstant(
      prometheusUrl,
      clusterMemoryCapacityPromql,
      timeoutMs,
    ),
    queryPrometheusInstant(
      prometheusUrl,
      clusterMemoryAvailablePromql,
      timeoutMs,
    ),
  ]);

  if (capacity_bytes === 0) {
    throw new Error("No cluster memory capacity data");
  }

  if (available_bytes > capacity_bytes) {
    throw new Error("Inconsistent cluster memory samples");
  }

  const used_bytes = capacity_bytes - available_bytes;

  return {
    available_bytes,
    capacity_bytes,
    used_bytes,
  };
}
