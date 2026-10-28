import type { MetricsSource } from "./metrics-source.js";
import { queryPrometheusInstantNumberAllowAbsent } from "./prometheus-instant-query.js";

export const podReadyPromql = 'sum(kube_pod_status_ready{condition="true"})';
export const podTotalPromql = "count(kube_pod_info)";
export const sandboxReadyPromql =
  "sum(max by (hypershell_cluster_id) (gateway_sandbox_ready))";
export const sandboxReadySourceAvailablePromql =
  "min(max by (hypershell_cluster_id) (gateway_sandbox_ready_source_available))";
export const sandboxTotalPromql =
  "sum(max by (hypershell_cluster_id, namespace) (gateway_sandbox_total))";

export interface ResourceReadinessMetrics {
  pod_readiness_percent?: number;
  sandbox_readiness_percent?: number;
  unready_resources?: number;
}

async function optionalNumber(
  source: MetricsSource,
  query: string,
  timeoutMs: number,
): Promise<number | undefined> {
  try {
    return await queryPrometheusInstantNumberAllowAbsent(
      source,
      query,
      timeoutMs,
    );
  } catch {
    return undefined;
  }
}

export async function queryResourceReadiness(
  podSource: MetricsSource,
  timeoutMs: number,
  sandboxSource: MetricsSource = podSource,
): Promise<ResourceReadinessMetrics> {
  const [
    podReady,
    podTotal,
    sandboxReady,
    sandboxTotal,
    sandboxSourceAvailable,
  ] = await Promise.all([
    optionalNumber(podSource, podReadyPromql, timeoutMs),
    optionalNumber(podSource, podTotalPromql, timeoutMs),
    optionalNumber(sandboxSource, sandboxReadyPromql, timeoutMs),
    optionalNumber(sandboxSource, sandboxTotalPromql, timeoutMs),
    optionalNumber(sandboxSource, sandboxReadySourceAvailablePromql, timeoutMs),
  ]);

  const validPods =
    podReady !== undefined &&
    podTotal !== undefined &&
    podTotal > 0 &&
    podReady <= podTotal;
  const validSandboxes =
    sandboxReady !== undefined &&
    sandboxTotal !== undefined &&
    sandboxSourceAvailable === 1 &&
    sandboxTotal > 0 &&
    sandboxReady <= sandboxTotal;

  return {
    ...(validPods
      ? { pod_readiness_percent: (100 * podReady) / podTotal }
      : {}),
    ...(validSandboxes
      ? { sandbox_readiness_percent: (100 * sandboxReady) / sandboxTotal }
      : {}),
    ...(validPods && validSandboxes
      ? {
          unready_resources:
            podTotal - podReady + (sandboxTotal - sandboxReady),
        }
      : {}),
  };
}
