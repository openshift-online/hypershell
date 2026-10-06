import type { useIntl } from "react-intl";

import {
  GatewayOperationError,
  type GatewayAccessCapabilities,
  type GatewayAccessGrantRecord,
  type GatewayAccessRole,
} from "../application/gateway-types";
import { messages } from "../messages";

type FormatMessage = ReturnType<typeof useIntl>["formatMessage"];

/** Ascending-privilege order used to render role pickers. */
export const accessRoleOrder: readonly GatewayAccessRole[] = [
  "user",
  "admin",
  "owner",
];

export function roleLabel(
  formatMessage: FormatMessage,
  role: GatewayAccessRole,
): string {
  const descriptor = {
    admin: messages.accessRoleAdmin,
    owner: messages.accessRoleOwner,
    user: messages.accessRoleUser,
  }[role];
  return formatMessage(descriptor);
}

export function roleDescription(
  formatMessage: FormatMessage,
  role: GatewayAccessRole,
): string {
  const descriptor = {
    admin: messages.accessRoleAdminDescription,
    owner: messages.accessRoleOwnerDescription,
    user: messages.accessRoleUserDescription,
  }[role];
  return formatMessage(descriptor);
}

/** Roles the caller may assign given their capabilities (GAM-08). */
export function assignableRoles(
  capabilities: GatewayAccessCapabilities,
): readonly GatewayAccessRole[] {
  if (!capabilities.canManageAccess) {
    return [];
  }
  return capabilities.canManageOwners
    ? accessRoleOrder
    : accessRoleOrder.filter((role) => role !== "owner");
}

/**
 * The accessible reason a row's management controls are disabled, or null when
 * the caller may act on the row. Mirrors the server guarantee (GAM-UI-08); the
 * server stays authoritative.
 *
 * ponytail: ownerCount is counted from the current page, so last-owner disabling
 * is best-effort when owners span pages; the server 409 is the backstop.
 */
export function rowDisabledReason(
  formatMessage: FormatMessage,
  grant: GatewayAccessGrantRecord,
  capabilities: GatewayAccessCapabilities,
  ownerCount: number,
): string | null {
  if (grant.role === "owner" && !capabilities.canManageOwners) {
    return formatMessage(messages.accessOwnerOnlyDisabled);
  }
  if (grant.role === "owner" && ownerCount <= 1) {
    return formatMessage(messages.accessLastOwnerDisabled);
  }
  return null;
}

/** Maps a failed access mutation to a localized, actionable message. */
export function actionErrorMessage(
  formatMessage: FormatMessage,
  error: unknown,
): string {
  if (error instanceof GatewayOperationError) {
    if (error.kind === "conflict") {
      return formatMessage(messages.accessActionErrorLastOwner);
    }
    if (error.kind === "denied") {
      return formatMessage(messages.accessActionErrorDenied);
    }
    if (error.kind === "not-found") {
      return formatMessage(messages.accessGrantErrorNotFound);
    }
  }
  return formatMessage(messages.accessActionErrorGeneric);
}
