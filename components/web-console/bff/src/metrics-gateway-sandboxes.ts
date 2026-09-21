import {
  applicationScalarQuery,
  queryPrometheusInstantScalar,
} from "./prometheus-instant-query.js";
import {
  alignDailyIntegerSeries,
  dailyRangeStepSeconds,
  hourlyRangeLookbackSeconds,
  hourlyRangeStepSeconds,
  mapHourlyIntegerSeries,
  queryPrometheusRangeScalarSamples,
  rollingHourlyRange,
  sevenDayUtcCalendarRange,
} from "./prometheus-range-query.js";
import { namespaceSelector, type MetricsSource } from "./metrics-source.js";

export const gatewayActiveSandboxesPromql =
  "hypershell_gateways_active_sandboxes_total";
/** Base control-plane OTLP gauge name (SSA-06); fleet PromQL wraps with sum(). */
export const gatewayOrphanedSandboxesPromql = "gateway_sandbox_orphaned";
export const gatewayExpiringSandboxesPromql = "gateway_sandbox_expiring";
export const gatewayIdleSandboxesPromql = "gateway_sandbox_idle";
export const gatewayActiveSandboxesHourlyStepSeconds = hourlyRangeStepSeconds;
export const gatewayActiveSandboxesHourlyLookbackSeconds =
  hourlyRangeLookbackSeconds;
export const gatewayActiveSandboxesDailyStepSeconds = dailyRangeStepSeconds;

/**
 * Fleet sum of control-plane attention gauges across scraped instances.
 * Uses `k8s_namespace_name` (OTLP resource attribute), not scrape `namespace`
 * (collector). `max by (hypershell_cluster_id)` dedupes control-plane replicas
 * for the same managed cluster before summing across clusters.
 */
export function attentionFleetQuery(
  metric: string,
  namespace?: string,
): string {
  const selector = namespaceSelector(namespace, "k8s_namespace_name");
  return `sum(max by (hypershell_cluster_id) (${metric}${selector}))`;
}

export interface GatewaySandboxesHourlyActive {
  count: number;
  hour: string;
}

export interface GatewaySandboxesDailyActive {
  count: number;
  date: string;
}

export interface GatewaySandboxesCounts {
  active_sandboxes: number;
  orphaned_sandboxes?: number;
  expiring_sandboxes?: number;
  idle_sandboxes?: number;
  daily_active_sandboxes?: GatewaySandboxesDailyActive[];
  hourly_active_sandboxes?: GatewaySandboxesHourlyActive[];
}

async function queryHourlyActiveSandboxes(
  prometheusUrl: MetricsSource,
  timeoutMs: number,
  namespace?: string,
): Promise<GatewaySandboxesHourlyActive[]> {
  const { end, start } = rollingHourlyRange();
  const query = applicationScalarQuery(gatewayActiveSandboxesPromql, namespace);
  const samples = await queryPrometheusRangeScalarSamples(
    prometheusUrl,
    query,
    start,
    end,
    `${String(gatewayActiveSandboxesHourlyStepSeconds)}s`,
    timeoutMs,
  );

  return mapHourlyIntegerSeries(samples).map(({ hour, count }) => ({
    count,
    hour,
  }));
}

async function queryDailyActiveSandboxes(
  prometheusUrl: MetricsSource,
  timeoutMs: number,
  namespace?: string,
): Promise<GatewaySandboxesDailyActive[]> {
  const { end, start } = sevenDayUtcCalendarRange();
  const query = applicationScalarQuery(gatewayActiveSandboxesPromql, namespace);
  const samples = await queryPrometheusRangeScalarSamples(
    prometheusUrl,
    query,
    start,
    end,
    `${String(gatewayActiveSandboxesDailyStepSeconds)}s`,
    timeoutMs,
  );

  return alignDailyIntegerSeries(samples).map(({ date, value }) => ({
    count: value,
    date,
  }));
}

export async function queryGatewaySandboxes(
  prometheusUrl: MetricsSource,
  timeoutMs: number,
  namespace?: string,
): Promise<GatewaySandboxesCounts> {
  const activeSandboxes = await queryPrometheusInstantScalar(
    prometheusUrl,
    applicationScalarQuery(gatewayActiveSandboxesPromql, namespace),
    timeoutMs,
  );

  const response: GatewaySandboxesCounts = {
    active_sandboxes: activeSandboxes,
  };

  // Query attention gauges in parallel so worst-case latency is one timeout,
  // not three stacked timeouts. Each failure still omits only that field.
  const [orphaned, expiring, idle] = await Promise.all([
    queryPrometheusInstantScalar(
      prometheusUrl,
      attentionFleetQuery(gatewayOrphanedSandboxesPromql, namespace),
      timeoutMs,
    ).catch(() => undefined),
    queryPrometheusInstantScalar(
      prometheusUrl,
      attentionFleetQuery(gatewayExpiringSandboxesPromql, namespace),
      timeoutMs,
    ).catch(() => undefined),
    queryPrometheusInstantScalar(
      prometheusUrl,
      attentionFleetQuery(gatewayIdleSandboxesPromql, namespace),
      timeoutMs,
    ).catch(() => undefined),
  ]);
  if (orphaned !== undefined) {
    response.orphaned_sandboxes = orphaned;
  }
  if (expiring !== undefined) {
    response.expiring_sandboxes = expiring;
  }
  if (idle !== undefined) {
    response.idle_sandboxes = idle;
  }

  try {
    response.hourly_active_sandboxes = await queryHourlyActiveSandboxes(
      prometheusUrl,
      timeoutMs,
      namespace,
    );
  } catch {
    // Omit hourly trend only.
  }

  try {
    response.daily_active_sandboxes = await queryDailyActiveSandboxes(
      prometheusUrl,
      timeoutMs,
      namespace,
    );
  } catch {
    // Omit daily trend only.
  }

  return response;
}
