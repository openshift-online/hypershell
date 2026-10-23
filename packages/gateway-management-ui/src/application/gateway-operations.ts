import type {
  GatewayAction,
  GatewayProbe,
  GatewayProbeOutcome,
  GatewayProbePublisher,
} from "./gateway-probes";
import {
  GatewayOperationError,
  type GatewayControlPlane,
  type GatewayInvocationContext,
  type GatewayOperations,
  type GatewayRecord,
  type GatewayVersionDrift,
  type GatewayWorkflowRuntime,
} from "./gateway-types";

export interface GatewayOperationDependencies {
  controlPlane: GatewayControlPlane;
  probes: GatewayProbePublisher;
  runtime: GatewayWorkflowRuntime;
}

interface ParsedGatewayVersion {
  // [major, minor, patch]
  readonly core: readonly [number, number, number];
  // Release-suffix identifiers, e.g. `rhaiv.7` -> ["rhaiv", 7]. Empty when the
  // tag has no suffix.
  readonly suffix: readonly (number | string)[];
}

// Matches a gateway version tag of the form vMAJOR.MINOR.PATCH[-suffix.N], for
// example `v0.1.2-rhaiv.7`. The leading `v` is optional and the release suffix
// is optional. A value that does not match is treated as unparseable.
const gatewayVersionPattern =
  /^v?(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z][0-9A-Za-z.-]*))?$/u;

/**
 * Parses a gateway version tag like `v0.1.2-rhaiv.7` into comparable parts.
 * Returns undefined when the value is missing or does not match the expected
 * shape, so callers can suppress drift on unparseable versions (WEB-VER-03).
 */
export function parseGatewayVersion(
  value: string | undefined,
): ParsedGatewayVersion | undefined {
  const trimmed = value?.trim() ?? "";
  if (!trimmed) {
    return undefined;
  }
  const match = gatewayVersionPattern.exec(trimmed);
  if (!match) {
    return undefined;
  }
  const [, major, minor, patch, rawSuffix] = match;
  const suffix = (rawSuffix ?? "")
    .split(".")
    .filter((identifier) => identifier.length > 0)
    .map((identifier) =>
      /^\d+$/u.test(identifier) ? Number(identifier) : identifier,
    );
  return {
    core: [Number(major), Number(minor), Number(patch)],
    suffix,
  };
}

function compareSuffixIdentifiers(
  left: number | string,
  right: number | string,
): number {
  const leftNumeric = typeof left === "number";
  const rightNumeric = typeof right === "number";
  if (leftNumeric && rightNumeric) {
    return left - right;
  }
  // Numeric identifiers have lower precedence than alphanumeric ones, matching
  // semantic-version release-suffix ordering.
  if (leftNumeric) {
    return -1;
  }
  if (rightNumeric) {
    return 1;
  }
  return left.localeCompare(right);
}

/**
 * Compares two parsed gateway versions. Returns a negative number when `left`
 * is older than `right`, zero when equal, and a positive number when newer. A
 * tag without a release suffix ranks above an otherwise equal tag that has one.
 */
export function compareGatewayVersions(
  left: ParsedGatewayVersion,
  right: ParsedGatewayVersion,
): number {
  const coreDifference =
    left.core[0] - right.core[0] ||
    left.core[1] - right.core[1] ||
    left.core[2] - right.core[2];
  if (coreDifference !== 0) {
    return coreDifference;
  }
  const leftSuffix = left.suffix;
  const rightSuffix = right.suffix;
  if (leftSuffix.length === 0 && rightSuffix.length === 0) {
    return 0;
  }
  // SemVer precedence: a version with no release suffix ranks above the same
  // core with a pre-release suffix. For a downstream respin (a bare `v0.1.2`
  // superseded by `v0.1.2-rhaiv.1`) this ordering is wrong, but that case is out
  // of scope here: both versions always come from suffixed `OPENSHELL_TAG` tags,
  // so a bare core never reaches this comparison in production.
  if (leftSuffix.length === 0) {
    return 1;
  }
  if (rightSuffix.length === 0) {
    return -1;
  }
  const shared = Math.min(leftSuffix.length, rightSuffix.length);
  for (let index = 0; index < shared; index += 1) {
    const leftIdentifier = leftSuffix[index];
    const rightIdentifier = rightSuffix[index];
    if (leftIdentifier === undefined || rightIdentifier === undefined) {
      break;
    }
    const difference = compareSuffixIdentifiers(
      leftIdentifier,
      rightIdentifier,
    );
    if (difference !== 0) {
      return difference;
    }
  }
  return leftSuffix.length - rightSuffix.length;
}

/**
 * Derives the newer-version-available verdict for a gateway (WEB-VER-03/WEB-VER-04).
 * The indicator fires (`"behind"`) only when the gateway tracks its cluster's
 * default image (not pinned), both the running and provisioning versions are
 * present and parseable, and the running version is strictly older than the
 * advertised provisioning version. Every other case suppresses it (`"none"`):
 * pinned image, a missing or unparseable value, or a running version equal to
 * or newer than the provisioning version.
 */
export function deriveGatewayVersionDrift(
  gateway: Pick<
    GatewayRecord,
    "gatewayVersion" | "imagePinned" | "provisioningVersion"
  >,
): GatewayVersionDrift {
  if (gateway.imagePinned === true) {
    return "none";
  }
  const running = parseGatewayVersion(gateway.gatewayVersion);
  const provisioning = parseGatewayVersion(gateway.provisioningVersion);
  if (!running || !provisioning) {
    return "none";
  }
  return compareGatewayVersions(running, provisioning) < 0 ? "behind" : "none";
}

function isCancelled(error: unknown): boolean {
  return (
    typeof error === "object" &&
    error !== null &&
    "name" in error &&
    error.name === "AbortError"
  );
}

function failureKind(error: unknown) {
  if (isCancelled(error)) {
    return "cancelled" as const;
  }
  return error instanceof GatewayOperationError ? error.kind : "unknown";
}

function failureOutcome(error: unknown): GatewayProbeOutcome {
  const kind = failureKind(error);
  if (kind === "cancelled") {
    return "cancelled";
  }
  if (kind === "conflict") {
    return "conflicted";
  }
  if (kind === "denied") {
    return "denied";
  }
  return "failed";
}

function probe(
  action: GatewayAction,
  correlationId: string,
  traceId: string,
  failure: ReturnType<typeof failureKind> | null,
  name: GatewayProbe["name"],
  occurredAt: string,
  outcome: GatewayProbeOutcome,
  operationId?: string,
  parentInvocationId?: string,
): GatewayProbe {
  return Object.freeze({
    context: Object.freeze({
      correlationId,
      traceId,
      ...(operationId === undefined ? {} : { operationId }),
      ...(parentInvocationId === undefined ? {} : { parentInvocationId }),
    }),
    fields: Object.freeze({ action, failureKind: failure, outcome }),
    name,
    occurredAt,
    schemaVersion: 1,
  });
}

export function createGatewayOperations({
  controlPlane,
  probes,
  runtime,
}: GatewayOperationDependencies): GatewayOperations {
  async function execute<T>(
    action: GatewayAction,
    signal: AbortSignal | undefined,
    task: (context: GatewayInvocationContext) => Promise<T>,
  ): Promise<T> {
    const correlationId = runtime.createCorrelationId();
    const traceId = runtime.createTraceId();
    const context: GatewayInvocationContext = {
      correlationId,
      ...(signal === undefined ? {} : { signal }),
    };
    probes.publish(
      probe(
        action,
        correlationId,
        traceId,
        null,
        "gateway.workflow.started",
        runtime.now(),
        "started",
      ),
    );
    probes.publish(
      probe(
        action,
        correlationId,
        traceId,
        null,
        "gateway.dependency.attempted",
        runtime.now(),
        "started",
        undefined,
        correlationId,
      ),
    );

    try {
      const result = await task(context);
      probes.publish(
        probe(
          action,
          correlationId,
          traceId,
          null,
          "gateway.dependency.completed",
          runtime.now(),
          "succeeded",
          undefined,
          correlationId,
        ),
      );
      probes.publish(
        probe(
          action,
          correlationId,
          traceId,
          null,
          "gateway.workflow.completed",
          runtime.now(),
          "succeeded",
        ),
      );
      return result;
    } catch (error) {
      const kind = failureKind(error);
      const outcome = failureOutcome(error);
      const operationId =
        error instanceof GatewayOperationError ? error.operationId : undefined;
      probes.publish(
        probe(
          action,
          correlationId,
          traceId,
          kind,
          "gateway.dependency.completed",
          runtime.now(),
          outcome,
          operationId,
          correlationId,
        ),
      );
      probes.publish(
        probe(
          action,
          correlationId,
          traceId,
          kind,
          "gateway.workflow.completed",
          runtime.now(),
          outcome,
          operationId,
        ),
      );
      throw error;
    }
  }

  return {
    changeGatewayAccessRole: (gatewayId, userId, role, signal) =>
      execute("change-access-role", signal, (context) =>
        controlPlane.changeGatewayAccessRole(gatewayId, userId, role, context),
      ),
    createOpenShellGatewayServiceAccount: (gatewayId, input, signal) =>
      execute("create-service-account", signal, (context) =>
        controlPlane.createOpenShellGatewayServiceAccount(
          gatewayId,
          input,
          context,
        ),
      ),
    deleteOpenShellGatewayServiceAccount: (
      gatewayId,
      serviceAccountId,
      signal,
    ) =>
      execute("delete-service-account", signal, (context) =>
        controlPlane.deleteOpenShellGatewayServiceAccount(
          gatewayId,
          serviceAccountId,
          context,
        ),
      ),
    findGatewayPlacements: (search, signal) =>
      execute("find-placements", signal, (context) =>
        controlPlane.findGatewayPlacements(search.trim(), context),
      ),
    getGatewayPlacementAvailability: (selectors, signal) =>
      execute("placement-availability", signal, (context) => {
        if (!controlPlane.getGatewayPlacementAvailability) {
          throw new GatewayOperationError("unknown", {
            cause: new Error("placement availability is unavailable"),
          });
        }
        return controlPlane.getGatewayPlacementAvailability(selectors, context);
      }),
    getGatewayPlacement: (clusterId, signal) =>
      execute("get-placement", signal, (context) =>
        controlPlane.getGatewayPlacement(clusterId, context),
      ),
    getGatewayPlacements: (clusterIds, signal) =>
      execute("get-placements", signal, (context) =>
        controlPlane.getGatewayPlacements(clusterIds, context),
      ),
    getGateway: (gatewayId, signal) =>
      execute("get", signal, (context) =>
        controlPlane.getGateway(gatewayId, context),
      ),
    grantGatewayAccess: (gatewayId, input, signal) =>
      execute("grant-access", signal, (context) =>
        controlPlane.grantGatewayAccess(gatewayId, input, context),
      ),
    listGatewayAccess: (gatewayId, request, signal) =>
      execute("list-access", signal, (context) =>
        controlPlane.listGatewayAccess(gatewayId, request, context),
      ),
    getOpenShellGatewayServiceAccount: (gatewayId, serviceAccountId, signal) =>
      execute("get-service-account", signal, (context) =>
        controlPlane.getOpenShellGatewayServiceAccount(
          gatewayId,
          serviceAccountId,
          context,
        ),
      ),
    listGateways: (request, signal) =>
      execute("list", signal, (context) =>
        controlPlane.listGateways(request, context),
      ),
    listOpenShellGatewayServiceAccounts: (gatewayId, request, signal) =>
      execute("list-service-accounts", signal, (context) =>
        controlPlane.listOpenShellGatewayServiceAccounts(
          gatewayId,
          request,
          context,
        ),
      ),
    provisionGateway: (input, signal) =>
      execute("provision", signal, (context) =>
        controlPlane.provisionGateway(input, context),
      ),
    removeGateway: (gatewayId, signal) =>
      execute("remove", signal, (context) =>
        controlPlane.removeGateway(gatewayId, context),
      ),
    renameGateway: (gatewayId, name, signal) =>
      execute("rename", signal, (context) =>
        controlPlane.renameGateway(gatewayId, name, context),
      ),
    revokeGatewayAccess: (gatewayId, userId, signal) =>
      execute("revoke-access", signal, (context) =>
        controlPlane.revokeGatewayAccess(gatewayId, userId, context),
      ),
    searchGatewayDirectory: (gatewayId, search, signal) =>
      execute("search-directory", signal, (context) =>
        controlPlane.searchGatewayDirectory(gatewayId, search.trim(), context),
      ),
    revokeOpenShellGatewayServiceAccount: (
      gatewayId,
      serviceAccountId,
      signal,
    ) =>
      execute("revoke-service-account", signal, (context) =>
        controlPlane.revokeOpenShellGatewayServiceAccount(
          gatewayId,
          serviceAccountId,
          context,
        ),
      ),
    updateOpenShellGatewayServiceAccountRole: (
      gatewayId,
      serviceAccountId,
      input,
      signal,
    ) =>
      execute("update-service-account-role", signal, (context) =>
        controlPlane.updateOpenShellGatewayServiceAccountRole(
          gatewayId,
          serviceAccountId,
          input,
          context,
        ),
      ),
  };
}
