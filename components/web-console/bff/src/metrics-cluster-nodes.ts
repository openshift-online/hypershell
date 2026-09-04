export const clusterNodesTotalPromql = "count(kube_node_info)";
export const clusterNodesReadyPromql =
  'sum(kube_node_status_condition{condition="Ready",status="true"})';

export interface ClusterNodesCounts {
  not_ready_nodes: number;
  ready_nodes: number;
  total_nodes: number;
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

export async function queryClusterNodes(
  prometheusUrl: string,
  timeoutMs: number,
): Promise<ClusterNodesCounts> {
  const [total_nodes, ready_nodes] = await Promise.all([
    queryPrometheusInstant(prometheusUrl, clusterNodesTotalPromql, timeoutMs),
    queryPrometheusInstant(prometheusUrl, clusterNodesReadyPromql, timeoutMs),
  ]);

  if (total_nodes === 0) {
    throw new Error("No cluster node data");
  }

  if (ready_nodes > total_nodes) {
    throw new Error("Inconsistent cluster node samples");
  }

  const not_ready_nodes = total_nodes - ready_nodes;

  return {
    not_ready_nodes,
    ready_nodes,
    total_nodes,
  };
}
