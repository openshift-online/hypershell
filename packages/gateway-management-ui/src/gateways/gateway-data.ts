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
// Upper bound on how long a settled routed gateway is polled for its console
// address, measured from when the UI first observes the gateway awaiting its
// console (see resolveConsoleWaitStart) -- NOT from the gateway's createdAt. A
// routed gateway is not proof of console eligibility: its console provisioning
// can be disabled, misconfigured, or stuck, in which case console_address never
// arrives. Without a bound the UI would poll that gateway every
// gatewayStatusPollMilliseconds forever. Once this window elapses, polling stops
// and the UI surfaces a terminal "console unavailable" state
// (gatewayConsoleUnavailable). Anchoring on the observed wait-start rather than
// createdAt keeps a pre-existing routed gateway -- or one first routed long after
// creation -- from being marked unavailable the instant it loads.
export const gatewayConsoleReadyDeadlineMilliseconds = 600_000;

// Canonical Gateway phase vocabulary emitted by the control plane, lowercased.
// Single source of truth on the console side, mirroring the Go gatewayhealth
// package (components/api-server/pkg/gatewayhealth). See
// specs/platform/gateway-phase-vocabulary.spec.md; keep in sync when the
// canonical phases change.
export const gatewayCanonicalPhases = {
  pending: "pending",
  provisioning: "provisioning",
  running: "running",
  degraded: "degraded",
  failed: "failed",
} as const;

// Recoverable (non-terminal) canonical phases keep the UI polling. The extra
// transitional descriptors (reconciling/updating) are tolerated in case they
// surface through the free-form health status, but the canonical phase set above
// is the source of truth for classification.
const gatewayPollingStates = new Set<string>([
  gatewayCanonicalPhases.pending,
  gatewayCanonicalPhases.provisioning,
  gatewayCanonicalPhases.degraded,
  "reconciling",
  "updating",
]);
// The canonical terminal-failure phase; "error" is tolerated from free-form
// status text.
const gatewayFailedLifecycleStates = new Set<string>([
  gatewayCanonicalPhases.failed,
  "error",
]);

type GatewayConsoleRecord = Pick<
  GatewayRecord,
  "phase" | "status" | "externalDns" | "consoleUrl"
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

// Resolves the timestamp from which a gateway's console-ready polling deadline is
// measured, given any timestamp the caller previously recorded for it. Returns
// the previously recorded start (or `now` on first observation) while the gateway
// is awaiting its console, and undefined once it no longer is -- console
// published, gateway failed, route removed, or still transitional -- so the
// caller can forget it and the clock restarts if the gateway becomes eligible
// again. Callers persist the returned value per gateway across polls, anchoring
// the deadline to when console-waiting actually began rather than to gateway
// creation; see useConsoleWaitTracker.
export function resolveConsoleWaitStart(
  gateway: GatewayConsoleRecord,
  now: number,
  previousStart: number | undefined,
): number | undefined {
  if (
    !gatewayAwaitingConsoleEligible(gateway, gatewayLifecycleStates(gateway))
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

  // Settled: keep polling only while still awaiting a console and within the
  // bounded wait window anchored on when console-waiting began.
  return (
    gatewayAwaitingConsoleEligible(gateway, states) &&
    withinConsoleReadyDeadline(consoleWaitStartedAt, now)
  );
}

// Reports whether the console-ready polling window is still open, given the
// timestamp when console-waiting began. An undefined or unparseable start cannot
// be bounded, so it is treated as outside the window (do not poll) rather than
// polled indefinitely; callers that must distinguish "not yet waiting" from
// "past the deadline" guard the undefined case themselves.
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
    id: gateway.id,
    name: gateway.name,
    ...(gateway.oidcAudience ? { oidcAudience: gateway.oidcAudience } : {}),
    ...(gateway.oidcClientId ? { oidcClientId: gateway.oidcClientId } : {}),
    ...(gateway.oidcIssuer ? { oidcIssuer: gateway.oidcIssuer } : {}),
    ...(phase ? { phase } : {}),
    status: status || "Unknown",
  };
}
