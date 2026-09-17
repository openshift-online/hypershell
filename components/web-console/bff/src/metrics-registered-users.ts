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
export const usersUniqueLoginsDailyPromql =
  "hypershell_users_unique_logins_daily_total";
export const usersUniqueLoginsLast7DaysPromql =
  "hypershell_users_unique_logins_last_7_days_total";
export const usersUniqueLoginsLast30DaysPromql =
  "hypershell_users_unique_logins_last_30_days_total";

export interface RegisteredUsersDailyLogin {
  date: string;
  count: number;
}

export interface RegisteredUsersResponse {
  total_registered: number;
  created_last_7_days?: number;
  created_last_30_days?: number;
  unique_logins_last_7_days?: number;
  unique_logins_last_30_days?: number;
  daily_unique_logins?: RegisteredUsersDailyLogin[];
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

export async function queryRegisteredUsers(
  prometheusUrl: MetricsSource,
  timeoutMs: number,
  namespace?: string,
): Promise<RegisteredUsersResponse> {
  const totalRegistered = await queryPrometheusInstantScalar(
    prometheusUrl,
    applicationScalarQuery(usersRegisteredTotalPromql, namespace),
    timeoutMs,
  );

  const [
    createdLast7Days,
    createdLast30Days,
    uniqueLoginsLast7Days,
    uniqueLoginsLast30Days,
    dailySamples,
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
    queryPrometheusInstantVector(
      prometheusUrl,
      applicationVectorQuery(
        usersUniqueLoginsDailyPromql,
        ["activity_date"],
        namespace,
      ),
      timeoutMs,
    ).catch(() => undefined),
  ]);

  const response: RegisteredUsersResponse = {
    total_registered: totalRegistered,
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

  if (dailySamples !== undefined) {
    const countsByDate = new Map<string, number>();
    for (const sample of dailySamples) {
      const date = sample.labels.activity_date;
      if (date) {
        countsByDate.set(date, sample.value);
      }
    }

    response.daily_unique_logins = utcCalendarDatesInclusive(30).map(
      (date) => ({
        date,
        count: countsByDate.get(date) ?? 0,
      }),
    );
  }

  return response;
}
