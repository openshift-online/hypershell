import type {
  GatewayListRequest,
  GatewayRecord,
} from "../application/gateway-types";
import { normalizeGatewayPlacementClusterIds } from "../application/gateway-placement";
import type { GatewayConnection } from "./gateway-connections";

export const gatewayListQueryRoot = ["gateways", "list"] as const;
export const gatewayPlacementQueryRoot = ["gateways", "placements"] as const;
export const gatewayPlacementStaleMilliseconds = 60_000;
export const gatewaySearchDebounceMilliseconds = 250;
export const gatewayStatusPollMilliseconds = 5_000;
// Upper bound for polling a settled routed gateway that needs its console
// address or runtime version. The clock starts when the UI first sees the
// missing data. It does not use the gateway creation time. This rule lets an
// existing gateway get a full wait window and prevents polling without an end.
export const gatewayConsoleReadyDeadlineMilliseconds = 600_000;

const gatewayPollingStates = new Set([
  "pending",
  "provisioning",
  "reconciling",
  "updating",
  "degraded",
]);
const gatewayFailedLifecycleStates = new Set(["error", "failed"]);

type GatewayConsoleRecord = Pick<
  GatewayRecord,
  "phase" | "status" | "externalDns" | "consoleUrl" | "gatewayVersion"
>;

// Lowercased, non-empty lifecycle states (phase and health status) of a gateway.
function gatewayLifecycleStates(gateway: GatewayConsoleRecord): string[] {
  return [gateway.phase, gateway.status]
    .map((value) => value?.trim().toLocaleLowerCase() ?? "")
    .filter(Boolean);
}

// True when a gateway has settled (reached a routed steady state, not
// transitional and not failed) with an external endpoint but no console URL yet:
// a per-gateway console reaches Running before its pod can serve, and the control
// plane publishes console_address only once that pod is Ready. This is the
// time-independent part of "awaiting a console"; the polling deadline is applied
// separately against the observed wait-start (withinConsoleReadyDeadline).
function gatewayAwaitingConsoleEligible(
  gateway: GatewayConsoleRecord,
  states: readonly string[],
): boolean {
  const routed = Boolean(gateway.externalDns?.trim());
  const consolePublished = Boolean(gateway.consoleUrl?.trim());
  const transitional =
    states.length === 0 ||
    states.some((value) => gatewayPollingStates.has(value));
  const failed = states.some((value) =>
    gatewayFailedLifecycleStates.has(value),
  );
  return routed && !consolePublished && !transitional && !failed;
}

// True when a routed gateway has settled but the control plane has not yet
// published the runtime version that the installation command needs.
function gatewayAwaitingVersionEligible(
  gateway: GatewayConsoleRecord,
  states: readonly string[],
): boolean {
  const routed = Boolean(gateway.externalDns?.trim());
  const versionPublished = Boolean(gateway.gatewayVersion?.trim());
  const transitional =
    states.length === 0 ||
    states.some((value) => gatewayPollingStates.has(value));
  const failed = states.some((value) =>
    gatewayFailedLifecycleStates.has(value),
  );
  return routed && !versionPublished && !transitional && !failed;
}

// Resolves the timestamp for the bounded wait for console connection data. The
// wait stays active while a settled routed gateway needs its console address or
// runtime version. It ends when both values arrive, or when the gateway becomes
// transitional, fails, or has no route.
export function resolveConsoleWaitStart(
  gateway: GatewayConsoleRecord,
  now: number,
  previousStart: number | undefined,
): number | undefined {
  const states = gatewayLifecycleStates(gateway);
  if (
    !gatewayAwaitingConsoleEligible(gateway, states) &&
    !gatewayAwaitingVersionEligible(gateway, states)
  ) {
    return undefined;
  }
  return previousStart ?? now;
}

export function gatewayNeedsStatusPolling(
  gateway: GatewayConsoleRecord,
  consoleWaitStartedAt?: number,
  now: number = Date.now(),
): boolean {
  const states = gatewayLifecycleStates(gateway);

  if (
    states.length === 0 ||
    states.some((value) => gatewayPollingStates.has(value))
  ) {
    return true;
  }

  // Keep polling during the bounded wait for a console address or runtime
  // version.
  return (
    (gatewayAwaitingConsoleEligible(gateway, states) ||
      gatewayAwaitingVersionEligible(gateway, states)) &&
    withinConsoleReadyDeadline(consoleWaitStartedAt, now)
  );
}

// Reports whether the connection-data polling window is still open. An invalid
// start time is outside the window, so polling cannot continue without a bound.
function withinConsoleReadyDeadline(
  consoleWaitStartedAt: number | undefined,
  now: number,
): boolean {
  if (
    consoleWaitStartedAt === undefined ||
    Number.isNaN(consoleWaitStartedAt)
  ) {
    return false;
  }
  return now - consoleWaitStartedAt < gatewayConsoleReadyDeadlineMilliseconds;
}

// Shared deadline primitive for views that already know a gateway is routed and
// settled (e.g. the detail header, gated by isGatewayReadyToConnect) and only
// need to distinguish "console still provisioning" from "console unavailable". A
// gateway not yet observed awaiting a console (undefined start) is still
// provisioning, not unavailable.
export function isGatewayConsolePastDeadline(
  consoleWaitStartedAt: number | undefined,
  now: number = Date.now(),
): boolean {
  if (consoleWaitStartedAt === undefined) {
    return false;
  }
  return !withinConsoleReadyDeadline(consoleWaitStartedAt, now);
}

// True when a routed gateway has settled (not transitional, not failed) without
// a console URL and the console-ready polling window (anchored on when
// console-waiting began) has elapsed, so the UI can surface a terminal "console
// unavailable" state instead of an indefinite "provisioning" spinner.
export function gatewayConsoleUnavailable(
  gateway: GatewayConsoleRecord,
  consoleWaitStartedAt?: number,
  now: number = Date.now(),
): boolean {
  const states = gatewayLifecycleStates(gateway);
  if (!gatewayAwaitingConsoleEligible(gateway, states)) {
    return false;
  }
  return !withinConsoleReadyDeadline(consoleWaitStartedAt, now);
}

export function gatewayListQueryKey(request: GatewayListRequest) {
  return [
    ...gatewayListQueryRoot,
    request.page,
    request.size,
    request.search,
    request.sortField,
    request.sortDirection,
  ] as const;
}

export function gatewayQueryKey(gatewayId: string) {
  return ["gateways", "detail", gatewayId] as const;
}

export function gatewayPlacementQueryKey(search: string) {
  return [...gatewayPlacementQueryRoot, "search", search.trim()] as const;
}

export function gatewayPlacementDetailQueryKey(clusterId: string) {
  return [...gatewayPlacementQueryRoot, "detail", clusterId] as const;
}

export function gatewayPlacementBatchQueryKey(clusterIds: readonly string[]) {
  const normalizedClusterIds = normalizeGatewayPlacementClusterIds(clusterIds);
  return [
    ...gatewayPlacementQueryRoot,
    "batch",
    ...normalizedClusterIds,
  ] as const;
}

type GatewayApiPayload = Omit<
  GatewayRecord,
  "externalDns" | "phase" | "status"
> &
  Partial<Pick<GatewayRecord, "externalDns" | "phase" | "status">>;

function gatewayEndpoint(gateway: GatewayApiPayload): string | undefined {
  const externalDns = gateway.externalDns?.trim() ?? "";
  if (!externalDns) {
    return undefined;
  }
  if (/^https?:\/\//iu.test(externalDns)) {
    return externalDns;
  }

  return `https://${externalDns}${externalDns.includes(":") ? "" : ":443"}`;
}

export function toGatewayConnection(
  gateway: GatewayApiPayload,
  hubClusterName: string,
): GatewayConnection {
  const clusterId = gateway.clusterId.trim();
  const phase = gateway.phase?.trim() ?? "";
  const healthStatus = gateway.status?.trim() ?? "";
  const normalizedPhase = phase.toLocaleLowerCase();
  const status =
    phase &&
    (gatewayPollingStates.has(normalizedPhase) ||
      gatewayFailedLifecycleStates.has(normalizedPhase))
      ? phase
      : healthStatus || phase;

  return {
    ...(typeof gateway.activeSandboxCount === "number"
      ? { activeSandboxCount: gateway.activeSandboxCount }
      : {}),
    ...(clusterId ? { clusterId } : {}),
    clusterName: clusterId ? "" : hubClusterName,
    ...(gateway.consoleUrl ? { consoleUrl: gateway.consoleUrl } : {}),
    ...(gateway.createdAt ? { createdAt: gateway.createdAt } : {}),
    ...(gateway.createdBy ? { createdBy: gateway.createdBy } : {}),
    endpoint: gatewayEndpoint(gateway),
    ...(gateway.gatewayVersion?.trim()
      ? { gatewayVersion: gateway.gatewayVersion.trim() }
      : {}),
    id: gateway.id,
    name: gateway.name,
    ...(gateway.oidcAudience ? { oidcAudience: gateway.oidcAudience } : {}),
    ...(gateway.oidcClientId ? { oidcClientId: gateway.oidcClientId } : {}),
    ...(gateway.oidcIssuer ? { oidcIssuer: gateway.oidcIssuer } : {}),
    ...(phase ? { phase } : {}),
    status: status || "Unknown",
  };
}
