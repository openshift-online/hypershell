import { SDKAPIError, type SDKClient } from "@openshift-online/hypershell-sdk";

import {
  PlatformPolicyOperationError,
  type PlatformPolicy,
  type PlatformPolicyControlPlane,
  type PlatformPolicyDiagnostic,
  type PlatformPolicyDraft,
  type PlatformPolicyEvaluation,
  type PlatformPolicyFailureKind,
  type PlatformPolicyGatewayStatus,
  type PlatformPolicyInvocationContext,
  type PlatformPolicySettings,
  type PlatformPolicyTarget,
  type PlatformPolicyTestRequest,
  type PlatformPolicyUpdate,
} from "../../application/platform-policies/platform-policy-types";
import {
  asArray,
  asBoolean,
  asNumber,
  asOptionalString,
  asRecord,
  asString,
  toFinding,
  toFindingCounts,
  toFleetDefaults,
  toPolicyMode,
  toRenderedRules,
  toRolloutCondition,
} from "./policy-mapping";
import type { PolicyTransportFactory } from "./policy-transport";

/*
 * API adapter for the platform policy admin panel (WEB-PADM-10). It maps the
 * policy endpoints to application-owned types and maps transport failures to
 * typed operation errors: 403 to denied (never retried), 409 to conflicted,
 * 400 to invalid with the server's validation reason.
 */

const policiesPath = "/api/hypershell/ext/platform_gateway_policies";
const settingsPath = "/api/hypershell/ext/platform_policy_settings";
const listPageSize = 100;

type GatewayNameApi = Pick<SDKClient["gateways"], "list">;
export type GatewayNameApiFactory = (correlationId: string) => {
  gateways: GatewayNameApi;
};

export function platformPolicyFailureKindForStatus(
  statusCode: number,
): PlatformPolicyFailureKind {
  if (statusCode === 400 || statusCode === 422) {
    return "invalid";
  }
  if (statusCode === 401 || statusCode === 403) {
    return "denied";
  }
  if (statusCode === 404) {
    return "not-found";
  }
  if (statusCode === 409) {
    return "conflicted";
  }
  if (statusCode === 408 || statusCode === 429 || statusCode >= 500) {
    return "unavailable";
  }
  return "unknown";
}

async function mapFailure<T>(task: () => Promise<T>): Promise<T> {
  try {
    return await task();
  } catch (error) {
    if (error instanceof SDKAPIError) {
      const kind = platformPolicyFailureKindForStatus(error.statusCode);
      throw new PlatformPolicyOperationError(kind, {
        cause: error,
        operationId: error.operationId || undefined,
        ...(kind === "invalid" && error.reason ? { reason: error.reason } : {}),
      });
    }
    throw error;
  }
}

function toTarget(value: unknown): PlatformPolicyTarget {
  const record = asRecord(value);
  const gatewayIds = asArray(record.gateway_ids).map(asString).filter(Boolean);
  return {
    all:
      record.all === true || (record.all === undefined && !gatewayIds.length),
    exclude: asArray(record.exclude).map(asString).filter(Boolean),
    gatewayIds,
  };
}

function toGatewayStatus(value: unknown): PlatformPolicyGatewayStatus {
  const record = asRecord(value);
  const appliedRevision = asOptionalString(record.applied_revision);
  const message = asOptionalString(record.message);
  const revision = asOptionalString(record.revision);
  return {
    ...(appliedRevision ? { appliedRevision } : {}),
    condition: toRolloutCondition(record.condition),
    effectiveMode: toPolicyMode(record.effective_mode),
    gatewayId: asString(record.gateway_id),
    ...(message ? { message } : {}),
    ...(revision ? { revision } : {}),
    wouldReject24h: asNumber(record.would_reject_24h),
  };
}

export function toPlatformPolicy(value: unknown): PlatformPolicy {
  const record = asRecord(value);
  const status = asRecord(record.status);
  const rollout = asRecord(status.rollout);
  const createdAt = asOptionalString(record.created_at);
  const description = asOptionalString(record.description);
  const documentationUrl = asOptionalString(record.documentation_url);
  const updatedAt = asOptionalString(record.updated_at);
  return {
    builtIn: asBoolean(record.built_in),
    cedar: asString(record.cedar),
    ...(createdAt ? { createdAt } : {}),
    ...(description ? { description } : {}),
    ...(documentationUrl ? { documentationUrl } : {}),
    effectiveMode: toPolicyMode(record.effective_mode),
    enforcementMode: toPolicyMode(record.enforcement_mode),
    findingsLast24h: toFindingCounts(status.findings_last_24h),
    gateways: asArray(status.gateways).map(toGatewayStatus),
    id: asString(record.id),
    name: asString(record.name),
    rendered: toRenderedRules(record.rendered),
    rollout: {
      applied: asNumber(rollout.applied),
      pending: asNumber(rollout.pending),
      unsupported: asNumber(rollout.unsupported),
    },
    target: toTarget(record.target),
    ...(updatedAt ? { updatedAt } : {}),
    userMessage: asString(record.user_message),
  };
}

export function toPlatformPolicySettings(
  value: unknown,
): PlatformPolicySettings {
  const record = asRecord(value);
  const updatedAt = asOptionalString(record.updated_at);
  const updatedBy = asOptionalString(record.updated_by);
  return {
    defaults: toFleetDefaults(record.defaults),
    mode: toPolicyMode(record.mode),
    schemaVersion: asString(record.schema_version),
    ...(updatedAt ? { updatedAt } : {}),
    ...(updatedBy ? { updatedBy } : {}),
  };
}

function toDiagnostic(value: unknown): PlatformPolicyDiagnostic {
  const record = asRecord(value);
  const suggestion = asOptionalString(record.suggestion);
  return {
    line: asNumber(record.line),
    message: asString(record.message),
    statement: asNumber(record.statement),
    ...(suggestion ? { suggestion } : {}),
  };
}

export function toPlatformPolicyEvaluation(
  value: unknown,
): PlatformPolicyEvaluation {
  const record = asRecord(value);
  return {
    defaults: toFleetDefaults(record.defaults),
    denialText: asString(record.denial_text),
    diagnostics: asArray(record.diagnostics).map(toDiagnostic),
    forbidsDefaults: asBoolean(record.forbids_defaults),
    rejectingStatements: asArray(record.rejecting_statements).map(asNumber),
    rendered: toRenderedRules(record.rendered),
    requestAllowed: asBoolean(record.sample_allowed),
    valid: asBoolean(record.valid),
  };
}

function targetBody(target: PlatformPolicyTarget) {
  return target.all
    ? { all: true, exclude: [...target.exclude] }
    : { all: false, exclude: [], gateway_ids: [...target.gatewayIds] };
}

function draftBody(draft: Partial<PlatformPolicyDraft>) {
  return {
    ...(draft.cedar === undefined ? {} : { cedar: draft.cedar }),
    ...(draft.description === undefined
      ? {}
      : { description: draft.description }),
    ...(draft.documentationUrl === undefined
      ? {}
      : { documentation_url: draft.documentationUrl }),
    ...(draft.target === undefined ? {} : { target: targetBody(draft.target) }),
    ...(draft.userMessage === undefined
      ? {}
      : { user_message: draft.userMessage }),
  };
}

function testRequestBody(request: PlatformPolicyTestRequest) {
  const quantity = (value: string | undefined) =>
    value === undefined || value.trim() === "" ? undefined : value.trim();
  const body: Record<string, unknown> = {};
  const assign = (key: string, value: unknown) => {
    if (value !== undefined) {
      body[key] = value;
    }
  };
  assign("cpu_request", quantity(request.cpuRequest));
  assign("cpu_limit", quantity(request.cpuLimit));
  assign("memory_request", quantity(request.memoryRequest));
  assign("memory_limit", quantity(request.memoryLimit));
  assign("gpu_count", request.gpuCount);
  assign("image", quantity(request.image));
  assign("sandbox_name", quantity(request.sandboxName));
  if (request.roles?.length) {
    body.roles = [...request.roles];
  }
  return body;
}

function escapeSearchLiteral(value: string): string {
  return value.replaceAll("'", "''");
}

export function createPlatformPolicyControlPlaneAdapter(
  transportFactory: PolicyTransportFactory,
  gatewayNameApiFactory?: GatewayNameApiFactory,
): PlatformPolicyControlPlane {
  const transport = (context: PlatformPolicyInvocationContext) =>
    transportFactory(context.correlationId);

  /**
   * Resolves gateway display names for rollout rows. `platform-policy:admin`
   * holds no gateway read authority (SEC-RBAC-30), so a denied lookup falls
   * back to gateway ids; any other failure propagates.
   */
  async function gatewayNames(
    ids: readonly string[],
    context: PlatformPolicyInvocationContext,
  ): Promise<ReadonlyMap<string, string>> {
    const unique = [...new Set(ids)].filter(Boolean);
    if (!gatewayNameApiFactory || unique.length === 0) {
      return new Map();
    }
    try {
      const response = await gatewayNameApiFactory(
        context.correlationId,
      ).gateways.list(
        {
          page: 1,
          search: `id in (${unique.map((id) => `'${escapeSearchLiteral(id)}'`).join(", ")})`,
          size: unique.length,
        },
        { signal: context.signal },
      );
      return new Map(
        response.items.map((gateway) => [gateway.id, gateway.name]),
      );
    } catch (error) {
      if (
        error instanceof SDKAPIError &&
        platformPolicyFailureKindForStatus(error.statusCode) === "denied"
      ) {
        return new Map();
      }
      throw error;
    }
  }

  async function withGatewayNames(
    policies: readonly PlatformPolicy[],
    context: PlatformPolicyInvocationContext,
  ): Promise<readonly PlatformPolicy[]> {
    const names = await gatewayNames(
      policies.flatMap((policy) =>
        policy.gateways.map((gateway) => gateway.gatewayId),
      ),
      context,
    );
    if (names.size === 0) {
      return policies;
    }
    return policies.map((policy) => ({
      ...policy,
      gateways: policy.gateways.map((gateway) => {
        const gatewayName = names.get(gateway.gatewayId);
        return gatewayName ? { ...gateway, gatewayName } : gateway;
      }),
    }));
  }

  return {
    createPolicy: (draft, context) =>
      mapFailure(async () =>
        toPlatformPolicy(
          await transport(context).request("POST", policiesPath, {
            body: { ...draftBody(draft), name: draft.name },
            signal: context.signal,
          }),
        ),
      ),
    deletePolicy: (policyId, context) =>
      mapFailure(async () => {
        await transport(context).request(
          "DELETE",
          `${policiesPath}/${encodeURIComponent(policyId)}`,
          { signal: context.signal },
        );
      }),
    evaluatePolicy: (input, context) =>
      mapFailure(async () =>
        toPlatformPolicyEvaluation(
          await transport(context).request("POST", `${policiesPath}/evaluate`, {
            body: {
              cedar: input.cedar,
              ...(input.name ? { name: input.name } : {}),
              ...(input.userMessage ? { user_message: input.userMessage } : {}),
              ...(input.documentationUrl
                ? { documentation_url: input.documentationUrl }
                : {}),
              ...(input.request
                ? { sample: testRequestBody(input.request) }
                : {}),
            },
            signal: context.signal,
          }),
        ),
      ),
    getPolicy: (policyId, context) =>
      mapFailure(async () => {
        const policy = toPlatformPolicy(
          await transport(context).request(
            "GET",
            `${policiesPath}/${encodeURIComponent(policyId)}`,
            { signal: context.signal },
          ),
        );
        const [named] = await withGatewayNames([policy], context);
        return named ?? policy;
      }),
    getSettings: (context) =>
      mapFailure(async () =>
        toPlatformPolicySettings(
          await transport(context).request("GET", settingsPath, {
            signal: context.signal,
          }),
        ),
      ),
    listFindings: (policyId, request, context) =>
      mapFailure(async () => {
        const query = new URLSearchParams({
          page: String(request.page),
          size: String(request.size),
        });
        if (request.gatewayId) {
          query.set("gateway_id", request.gatewayId);
        }
        if (request.outcome) {
          query.set(
            "outcome",
            request.outcome === "would-reject"
              ? "would_reject"
              : "would_add_defaults",
          );
        }
        const record = asRecord(
          await transport(context).request(
            "GET",
            `${policiesPath}/${encodeURIComponent(policyId)}/findings?${query.toString()}`,
            { signal: context.signal },
          ),
        );
        return {
          items: asArray(record.items).map(toFinding),
          page: asNumber(record.page) || request.page,
          size: asNumber(record.size) || request.size,
          total: asNumber(record.total),
        };
      }),
    listPolicies: (context) =>
      mapFailure(async () => {
        const policies: PlatformPolicy[] = [];
        for (let page = 1; ; page += 1) {
          const record = asRecord(
            await transport(context).request(
              "GET",
              `${policiesPath}?page=${String(page)}&size=${String(listPageSize)}&orderBy=name%20asc`,
              { signal: context.signal },
            ),
          );
          const items = asArray(record.items).map(toPlatformPolicy);
          policies.push(...items);
          if (items.length === 0 || policies.length >= asNumber(record.total)) {
            break;
          }
        }
        return withGatewayNames(policies, context);
      }),
    updatePolicy: (policyId, update: PlatformPolicyUpdate, context) =>
      mapFailure(async () =>
        toPlatformPolicy(
          await transport(context).request(
            "PATCH",
            `${policiesPath}/${encodeURIComponent(policyId)}`,
            {
              body: {
                ...draftBody(update),
                ...(update.enforcementMode
                  ? { enforcement_mode: update.enforcementMode }
                  : {}),
              },
              signal: context.signal,
            },
          ),
        ),
      ),
    updateSettings: (mode, context) =>
      mapFailure(async () =>
        toPlatformPolicySettings(
          await transport(context).request("PATCH", settingsPath, {
            body: { mode },
            signal: context.signal,
          }),
        ),
      ),
  };
}
