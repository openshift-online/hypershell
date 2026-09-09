export const gatewayProvisionDurationCountPromql =
  "gateway_provision_duration_seconds_count";
export const gatewayProvisionDurationMeanPromql =
  "gateway_provision_duration_seconds_sum / gateway_provision_duration_seconds_count";
export const gatewayProvisionDurationP50Promql =
  "histogram_quantile(0.50, sum(gateway_provision_duration_seconds_bucket) by (le))";
export const gatewayProvisionDurationP95Promql =
  "histogram_quantile(0.95, sum(gateway_provision_duration_seconds_bucket) by (le))";

export interface GatewayProvisionDurationSeconds {
  mean_seconds: number;
  observation_count: number;
  p50_seconds: number;
  p95_seconds: number;
}

interface PrometheusQueryResponse {
  status: string;
  data?: {
    result: {
      value: [string, string];
    }[];
  };
}

async function queryPrometheusInstantNumber(
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

    return value;
  } finally {
    clearTimeout(timeout);
  }
}

export async function queryGatewayProvisionDuration(
  prometheusUrl: string,
  timeoutMs: number,
): Promise<GatewayProvisionDurationSeconds> {
  const observation_count = Math.round(
    await queryPrometheusInstantNumber(
      prometheusUrl,
      gatewayProvisionDurationCountPromql,
      timeoutMs,
    ),
  );

  if (observation_count === 0) {
    throw new Error("No gateway provision duration observations");
  }

  const [mean_seconds, p50_seconds, p95_seconds] = await Promise.all([
    queryPrometheusInstantNumber(
      prometheusUrl,
      gatewayProvisionDurationMeanPromql,
      timeoutMs,
    ),
    queryPrometheusInstantNumber(
      prometheusUrl,
      gatewayProvisionDurationP50Promql,
      timeoutMs,
    ),
    queryPrometheusInstantNumber(
      prometheusUrl,
      gatewayProvisionDurationP95Promql,
      timeoutMs,
    ),
  ]);

  return {
    mean_seconds,
    observation_count,
    p50_seconds,
    p95_seconds,
  };
}
