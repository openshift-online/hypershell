/** Keycloak realm role for HyperShell administrators. */
export const HYPERSHELL_ADMIN_ROLE = "hypershell-admins";

/** Keycloak realm role for platform-wide administration. */
export const PLATFORM_ADMIN_ROLE = "platform:admin";

const DASHBOARD_ADMIN_ROLES = new Set([
  HYPERSHELL_ADMIN_ROLE,
  PLATFORM_ADMIN_ROLE,
]);

export function hasDashboardAdminRole(roles: readonly string[]): boolean {
  return roles.some((role) => DASHBOARD_ADMIN_ROLES.has(role));
}
