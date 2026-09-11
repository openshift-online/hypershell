import {
  applicationScalarQuery,
  queryPrometheusInstantScalar,
} from "./prometheus-instant-query.js";
import type { MetricsSource } from "./metrics-source.js";

export const usersRegisteredTotalPromql = "hypershell_users_registered_total";

export interface RegisteredUsersResponse {
  total_registered: number;
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

  return { total_registered: totalRegistered };
}
