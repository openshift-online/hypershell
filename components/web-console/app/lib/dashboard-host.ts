export const DASHBOARD_HOST_PREFIX = "dashboard.";

export function isDashboardHost(hostname: string): boolean {
  return hostname.startsWith(DASHBOARD_HOST_PREFIX);
}
