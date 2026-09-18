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
import type { MetricsSource } from "./metrics-source.js";

export const gatewayActiveSandboxesPromql =
  "hypershell_gateways_active_sandboxes_total";
export const gatewayActiveSandboxesHourlyStepSeconds = hourlyRangeStepSeconds;
export const gatewayActiveSandboxesHourlyLookbackSeconds =
  hourlyRangeLookbackSeconds;
export const gatewayActiveSandboxesDailyStepSeconds = dailyRangeStepSeconds;

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
