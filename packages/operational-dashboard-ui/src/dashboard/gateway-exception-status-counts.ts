import type { OperationalMetricStatus } from "../application/dashboard-types";

export function getGatewayExceptionStatusCounts(
  status: OperationalMetricStatus,
): Readonly<{ degraded: number; failed: number }> {
  return {
    degraded: status.degraded ?? 0,
    failed: status.failed ?? 0,
  };
}
