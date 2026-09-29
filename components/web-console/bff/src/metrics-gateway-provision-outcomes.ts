import {
  fetchMetrics,
  fetchMetricsRange,
  namespaceSelector,
  type MetricsSource,
} from "./metrics-source.js";

export const gatewayProvisionOutcomesSuccess24hPromql =
  'sum(increase(gateway_provision_outcomes_total{outcome="success"}[24h]))';
export const gatewayProvisionOutcomesFailure24hPromql =
  'sum(increase(gateway_provision_outcomes_total{outcome="failure"}[24h]))';
export const gatewayProvisionOutcomesSuccessHourlyPromql =
  'sum(increase(gateway_provision_outcomes_total{outcome="success"}[1h]))';
export const gatewayProvisionOutcomesFailureHourlyPromql =
  'sum(increase(gateway_provision_outcomes_total{outcome="failure"}[1h]))';
export const gatewayProvisionDurationCount24hPromql =
  "sum(increase(gateway_provision_duration_seconds_count[24h]))";
export const gatewayProvisionDurationCountLifetimePromql =
  "sum(gateway_provision_duration_seconds_count)";
export const gatewayProvisionDurationCountHourlyPromql =
  "sum(increase(gateway_provision_duration_seconds_count[1h]))";

export interface GatewayProvisionHourlySuccessRate {
  failure_count: number;
  hour: string;
  success_count: number;
  success_rate_percent: number;
}

export interface GatewayProvisionOutcomes {
  failure_count_24h: number;
  hourly_success_rate: GatewayProvisionHourlySuccessRate[];
  success_count_24h: number;
  success_rate_percent: number | null;
}

interface PrometheusQueryResponse {
  status: string;
  data?: {
    result: {
      value: [string, string];
    }[];
  };
}

interface PrometheusRangeQueryResponse {
  status: string;
  data?: {
    result: {
      values: [string, string][];
    }[];
  };
}

const hourlyRangeSeconds = 24 * 60 * 60;
const hourlyStepSeconds = 60 * 60;

function roundCount(value: number): number {
  return Math.round(value);
}

function computeSuccessRatePercent(
  successCount: number,
  failureCount: number,
): number | null {
  const total = successCount + failureCount;
  if (total <= 0) {
    return null;
  }

  return Math.round((successCount / total) * 1000) / 10;
}

function formatHourLabel(unixSeconds: number): string {
  const date = new Date(unixSeconds * 1000);
  const year = date.getUTCFullYear();
  const month = String(date.getUTCMonth() + 1).padStart(2, "0");
  const day = String(date.getUTCDate()).padStart(2, "0");
  const hour = String(date.getUTCHours()).padStart(2, "0");

  return `${String(year)}-${month}-${day}T${hour}:00`;
}

async function queryPrometheusInstantNumber(
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
      return 0;
    }

    const rawValue = samples[0]?.value[1];
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

async function queryPrometheusHourlyIncreases(
  prometheusUrl: MetricsSource,
  query: string,
  timeoutMs: number,
): Promise<Map<number, number>> {
  const controller = new AbortController();
  const timeoutReason = new Error("Prometheus query timed out");
  const timeout = setTimeout(() => {
    controller.abort(timeoutReason);
  }, timeoutMs);

  try {
    const end = Math.floor(Date.now() / 1000);
    const start = end - hourlyRangeSeconds;
    const response = await fetchMetricsRange(
      prometheusUrl,
      query,
      start,
      end,
      `${String(hourlyStepSeconds)}s`,
      controller.signal,
    );
    if (!response.ok) {
      throw new Error("Prometheus range query request failed");
    }

    const body = (await response.json()) as PrometheusRangeQueryResponse;
    if (body.status !== "success") {
      throw new Error("Prometheus range query returned non-success status");
    }

    const values = body.data?.result[0]?.values ?? [];
    const increases = new Map<number, number>();

    for (const [timestamp, rawValue] of values) {
      const value = Number(rawValue);
      if (!Number.isFinite(value) || value < 0) {
        throw new Error("Prometheus range query returned invalid sample");
      }

      increases.set(Number(timestamp), value);
    }

    return increases;
  } finally {
    clearTimeout(timeout);
  }
}

function buildHourlySuccessRate(
  successIncreases: Map<number, number>,
  failureIncreases: Map<number, number>,
): GatewayProvisionHourlySuccessRate[] {
  const timestamps = new Set([
    ...successIncreases.keys(),
    ...failureIncreases.keys(),
  ]);
  const hourly: GatewayProvisionHourlySuccessRate[] = [];

  for (const timestamp of [...timestamps].sort((left, right) => left - right)) {
    const successCount = roundCount(successIncreases.get(timestamp) ?? 0);
    const failureCount = roundCount(failureIncreases.get(timestamp) ?? 0);
    const successRatePercent = computeSuccessRatePercent(
      successCount,
      failureCount,
    );

    if (successRatePercent === null) {
      continue;
    }

    hourly.push({
      failure_count: failureCount,
      hour: formatHourLabel(timestamp),
      success_count: successCount,
      success_rate_percent: successRatePercent,
    });
  }

  return hourly;
}

function outcomeMetricSelector(
  namespace: string | undefined,
  outcome: "failure" | "success",
): string {
  if (!namespace) {
    return `{outcome="${outcome}"}`;
  }

  const namespaceLabel = namespaceSelector(
    namespace,
    "k8s_namespace_name",
  ).slice(1, -1);

  return `{${namespaceLabel},outcome="${outcome}"}`;
}

function durationCountMetric(namespace?: string): string {
  if (!namespace) {
    return "gateway_provision_duration_seconds_count";
  }

  return `gateway_provision_duration_seconds_count${namespaceSelector(namespace, "k8s_namespace_name")}`;
}

export async function queryGatewayProvisionOutcomes(
  prometheusUrl: MetricsSource,
  timeoutMs: number,
  namespace?: string,
): Promise<GatewayProvisionOutcomes> {
  const successSelector = outcomeMetricSelector(namespace, "success");
  const failureSelector = outcomeMetricSelector(namespace, "failure");
  const durationCountMetricName = durationCountMetric(namespace);
  const success24hQuery = `sum(increase(gateway_provision_outcomes_total${successSelector}[24h]))`;
  const failure24hQuery = `sum(increase(gateway_provision_outcomes_total${failureSelector}[24h]))`;
  const successHourlyQuery = `sum(increase(gateway_provision_outcomes_total${successSelector}[1h]))`;
  const failureHourlyQuery = `sum(increase(gateway_provision_outcomes_total${failureSelector}[1h]))`;
  const durationCount24hQuery = `sum(increase(${durationCountMetricName}[24h]))`;
  const durationCountLifetimeQuery = `sum(${durationCountMetricName})`;
  const durationCountHourlyQuery = `sum(increase(${durationCountMetricName}[1h]))`;

  let successCount24h = roundCount(
    await queryPrometheusInstantNumber(
      prometheusUrl,
      success24hQuery,
      timeoutMs,
    ),
  );
  let failureCount24h = roundCount(
    await queryPrometheusInstantNumber(
      prometheusUrl,
      failure24hQuery,
      timeoutMs,
    ),
  );

  let successHourlyIncreases: Map<number, number>;
  let failureHourlyIncreases: Map<number, number>;

  if (successCount24h === 0 && failureCount24h === 0) {
    const durationCount24h = roundCount(
      await queryPrometheusInstantNumber(
        prometheusUrl,
        durationCount24hQuery,
        timeoutMs,
      ),
    );

    if (durationCount24h > 0) {
      successCount24h = durationCount24h;
      failureCount24h = 0;
      successHourlyIncreases = await queryPrometheusHourlyIncreases(
        prometheusUrl,
        durationCountHourlyQuery,
        timeoutMs,
      );
      failureHourlyIncreases = new Map();
    } else {
      const lifetimeSuccessCount = roundCount(
        await queryPrometheusInstantNumber(
          prometheusUrl,
          durationCountLifetimeQuery,
          timeoutMs,
        ),
      );

      if (lifetimeSuccessCount > 0) {
        successCount24h = lifetimeSuccessCount;
        failureCount24h = 0;
        successHourlyIncreases = new Map();
        failureHourlyIncreases = new Map();
      } else {
        successHourlyIncreases = new Map();
        failureHourlyIncreases = new Map();
      }
    }
  } else {
    successHourlyIncreases = await queryPrometheusHourlyIncreases(
      prometheusUrl,
      successHourlyQuery,
      timeoutMs,
    );
    failureHourlyIncreases = await queryPrometheusHourlyIncreases(
      prometheusUrl,
      failureHourlyQuery,
      timeoutMs,
    );
  }

  const successRatePercent = computeSuccessRatePercent(
    successCount24h,
    failureCount24h,
  );

  return {
    failure_count_24h: failureCount24h,
    hourly_success_rate: buildHourlySuccessRate(
      successHourlyIncreases,
      failureHourlyIncreases,
    ),
    success_count_24h: successCount24h,
    success_rate_percent: successRatePercent,
  };
}
