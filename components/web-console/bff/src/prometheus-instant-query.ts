import {
  fetchMetrics,
  namespaceSelector,
  type MetricsSource,
} from "./metrics-source.js";

interface PrometheusQueryResponse {
  status: string;
  data?: {
    result: {
      metric: Record<string, string>;
      value: [string, string];
    }[];
  };
}

export interface PrometheusLabeledSample {
  labels: Record<string, string>;
  value: number;
}

/** Build a scalar application-metric query, deduplicating API replica series. */
export function applicationScalarQuery(
  metric: string,
  namespace?: string,
): string {
  if (!namespace) {
    return metric;
  }
  return `max(${metric}${namespaceSelector(namespace)})`;
}

/** Build a labeled application-metric query, deduplicating API replica series. */
export function applicationVectorQuery(
  metric: string,
  groupBy: readonly string[],
  namespace?: string,
): string {
  if (!namespace) {
    return metric;
  }
  const labels = groupBy.join(", ");
  return `max by (${labels}) (${metric}${namespaceSelector(namespace)})`;
}

async function queryPrometheus(
  source: MetricsSource,
  query: string,
  timeoutMs: number,
): Promise<PrometheusQueryResponse> {
  const controller = new AbortController();
  const timeoutReason = new Error("Prometheus query timed out");
  const timeout = setTimeout(() => {
    controller.abort(timeoutReason);
  }, timeoutMs);

  try {
    const response = await fetchMetrics(source, query, controller.signal);
    if (!response.ok) {
      throw new Error("Prometheus query request failed");
    }

    const body = (await response.json()) as PrometheusQueryResponse;
    if (body.status !== "success") {
      throw new Error("Prometheus query returned non-success status");
    }

    return body;
  } finally {
    clearTimeout(timeout);
  }
}

function parseSampleValue(rawValue: string | undefined): number {
  if (rawValue === undefined) {
    throw new Error("Prometheus query returned invalid sample");
  }

  const value = Number(rawValue);
  if (!Number.isFinite(value) || value < 0) {
    throw new Error("Prometheus query returned invalid sample");
  }

  return value;
}

export async function queryPrometheusInstantScalar(
  source: MetricsSource,
  query: string,
  timeoutMs: number,
): Promise<number> {
  const body = await queryPrometheus(source, query, timeoutMs);
  const samples = body.data?.result ?? [];
  if (samples.length === 0) {
    throw new Error("Prometheus query returned no samples");
  }

  let maxValue = 0;
  for (const sample of samples) {
    maxValue = Math.max(
      maxValue,
      Math.round(parseSampleValue(sample.value[1])),
    );
  }

  return maxValue;
}

export async function queryPrometheusInstantVector(
  source: MetricsSource,
  query: string,
  timeoutMs: number,
): Promise<PrometheusLabeledSample[]> {
  const body = await queryPrometheus(source, query, timeoutMs);

  const samplesByLabels = new Map<string, PrometheusLabeledSample>();
  for (const sample of body.data?.result ?? []) {
    const value = Math.round(parseSampleValue(sample.value[1]));
    const key = JSON.stringify(sample.metric);
    const existing = samplesByLabels.get(key);
    if (existing === undefined) {
      samplesByLabels.set(key, { labels: sample.metric, value });
      continue;
    }
    existing.value = Math.max(existing.value, value);
  }

  return [...samplesByLabels.values()];
}
