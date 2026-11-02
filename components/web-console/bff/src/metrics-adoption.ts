import {
  applicationScalarQuery,
  applicationVectorQuery,
  queryPrometheusInstantScalar,
  queryPrometheusInstantVector,
} from "./prometheus-instant-query.js";
import type { MetricsSource } from "./metrics-source.js";

export const usersRegisteredTotalPromql = "hypershell_users_registered_total";
export const usersCreatedLast7DaysPromql =
  "hypershell_users_created_last_7_days_total";
export const usersCreatedLast30DaysPromql =
  "hypershell_users_created_last_30_days_total";
export const usersCreatedDailyPromql = "hypershell_users_created_daily_total";
export const usersInactiveLast30DaysPromql =
  "hypershell_users_inactive_last_30_days_total";
export const usersUniqueLoginsDailyPromql =
  "hypershell_users_unique_logins_daily_total";
export const usersUniqueLoginsLast7DaysPromql =
  "hypershell_users_unique_logins_last_7_days_total";
export const usersUniqueLoginsLast30DaysPromql =
  "hypershell_users_unique_logins_last_30_days_total";
export const usersRetention7DaysPromql =
  "hypershell_users_retention_7_days_percent";
export const gatewayOwnerUsersPromql =
  "hypershell_role_bindings_gateway_owner_users_total";

export interface AdoptionDailyPoint {
  date: string;
  count: number;
}

export interface AdoptionMetricsResponse {
  registered_users: number;
  created_last_7_days?: number;
  created_last_30_days?: number;
  unique_logins_last_7_days?: number;
  unique_logins_last_30_days?: number;
  inactive_users_last_30_days?: number;
  users_with_gateway?: number;
  gateway_conversion_percent?: number;
  daily_created_users?: AdoptionDailyPoint[];
  daily_unique_logins?: AdoptionDailyPoint[];
  retention_7_days_percent?: number;
}

function utcCalendarDatesInclusive(days: number): string[] {
  const dates: string[] = [];
  const today = new Date();
  const utcToday = Date.UTC(
    today.getUTCFullYear(),
    today.getUTCMonth(),
    today.getUTCDate(),
  );

  for (let offset = days - 1; offset >= 0; offset -= 1) {
    const day = new Date(utcToday - offset * 24 * 60 * 60 * 1000);
    dates.push(day.toISOString().slice(0, 10));
  }

  return dates;
}

async function queryOptionalScalar(
  prometheusUrl: MetricsSource,
  promql: string,
  timeoutMs: number,
  namespace?: string,
): Promise<number | undefined> {
  try {
    return await queryPrometheusInstantScalar(
      prometheusUrl,
      applicationScalarQuery(promql, namespace),
      timeoutMs,
    );
  } catch {
    return undefined;
  }
}

async function queryOptionalDailySeries(
  prometheusUrl: MetricsSource,
  promql: string,
  dateLabel: string,
  timeoutMs: number,
  namespace?: string,
): Promise<AdoptionDailyPoint[] | undefined> {
  try {
    const samples = await queryPrometheusInstantVector(
      prometheusUrl,
      applicationVectorQuery(promql, [dateLabel], namespace),
      timeoutMs,
    );
    const countsByDate = new Map<string, number>();
    for (const sample of samples) {
      const date = sample.labels[dateLabel];
      if (date) {
        countsByDate.set(date, sample.value);
      }
    }

    return utcCalendarDatesInclusive(30).map((date) => ({
      date,
      count: countsByDate.get(date) ?? 0,
    }));
  } catch {
    return undefined;
  }
}

export async function queryAdoptionMetrics(
  prometheusUrl: MetricsSource,
  timeoutMs: number,
  namespace?: string,
): Promise<AdoptionMetricsResponse> {
  const registeredUsers = await queryPrometheusInstantScalar(
    prometheusUrl,
    applicationScalarQuery(usersRegisteredTotalPromql, namespace),
    timeoutMs,
  );

  const [
    createdLast7Days,
    createdLast30Days,
    uniqueLoginsLast7Days,
    uniqueLoginsLast30Days,
    inactiveUsersLast30Days,
    usersWithGateway,
    retention7DaysPercent,
    dailyCreatedUsers,
    dailyUniqueLogins,
  ] = await Promise.all([
    queryOptionalScalar(
      prometheusUrl,
      usersCreatedLast7DaysPromql,
      timeoutMs,
      namespace,
    ),
    queryOptionalScalar(
      prometheusUrl,
      usersCreatedLast30DaysPromql,
      timeoutMs,
      namespace,
    ),
    queryOptionalScalar(
      prometheusUrl,
      usersUniqueLoginsLast7DaysPromql,
      timeoutMs,
      namespace,
    ),
    queryOptionalScalar(
      prometheusUrl,
      usersUniqueLoginsLast30DaysPromql,
      timeoutMs,
      namespace,
    ),
    queryOptionalScalar(
      prometheusUrl,
      usersInactiveLast30DaysPromql,
      timeoutMs,
      namespace,
    ),
    queryOptionalScalar(
      prometheusUrl,
      gatewayOwnerUsersPromql,
      timeoutMs,
      namespace,
    ),
    queryOptionalScalar(
      prometheusUrl,
      usersRetention7DaysPromql,
      timeoutMs,
      namespace,
    ),
    queryOptionalDailySeries(
      prometheusUrl,
      usersCreatedDailyPromql,
      "created_date",
      timeoutMs,
      namespace,
    ),
    queryOptionalDailySeries(
      prometheusUrl,
      usersUniqueLoginsDailyPromql,
      "activity_date",
      timeoutMs,
      namespace,
    ),
  ]);

  const response: AdoptionMetricsResponse = {
    registered_users: registeredUsers,
  };

  if (createdLast7Days !== undefined) {
    response.created_last_7_days = createdLast7Days;
  }
  if (createdLast30Days !== undefined) {
    response.created_last_30_days = createdLast30Days;
  }
  if (uniqueLoginsLast7Days !== undefined) {
    response.unique_logins_last_7_days = uniqueLoginsLast7Days;
  }
  if (uniqueLoginsLast30Days !== undefined) {
    response.unique_logins_last_30_days = uniqueLoginsLast30Days;
  }
  if (inactiveUsersLast30Days !== undefined) {
    response.inactive_users_last_30_days = inactiveUsersLast30Days;
  }
  if (usersWithGateway !== undefined) {
    response.users_with_gateway = usersWithGateway;
    if (registeredUsers > 0) {
      response.gateway_conversion_percent =
        Math.round((usersWithGateway / registeredUsers) * 1000) / 10;
    }
  }
  if (retention7DaysPercent !== undefined) {
    response.retention_7_days_percent = retention7DaysPercent;
  }
  if (dailyCreatedUsers !== undefined) {
    response.daily_created_users = dailyCreatedUsers;
  }
  if (dailyUniqueLogins !== undefined) {
    response.daily_unique_logins = dailyUniqueLogins;
  }

  return response;
}
