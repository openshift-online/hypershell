import type {
  EffectivePolicy,
  GatewayEffectivePolicies,
  PolicyBound,
  PolicyFinding,
  PolicyFindingCounts,
  PolicyFleetDefaults,
  PolicyMode,
  PolicyModeReason,
  PolicyRenderedRule,
  PolicyRolloutCondition,
} from "@openshift-online/hypershell-gateway-management-ui";

/*
 * Maps platform policy transport records (snake_case JSON) to the
 * application-owned policy types. Every field is read defensively so a partial
 * or newer response degrades to an explicit empty value rather than throwing
 * inside a view.
 */

export type JsonRecord = Readonly<Record<string, unknown>>;

export function asRecord(value: unknown): JsonRecord {
  return typeof value === "object" && value !== null && !Array.isArray(value)
    ? (value as JsonRecord)
    : {};
}

export function asArray(value: unknown): readonly unknown[] {
  return Array.isArray(value) ? value : [];
}

export function asString(value: unknown): string {
  return typeof value === "string" ? value : "";
}

export function asOptionalString(value: unknown): string | undefined {
  return typeof value === "string" && value !== "" ? value : undefined;
}

export function asNumber(value: unknown): number {
  return typeof value === "number" && Number.isFinite(value) ? value : 0;
}

export function asBoolean(value: unknown): boolean {
  return value === true;
}

export function toPolicyMode(value: unknown): PolicyMode {
  return value === "enforce" ? "enforce" : "audit";
}

export function toPolicyModeReason(value: unknown): PolicyModeReason {
  if (value === "platform_enforced") {
    return "platform-enforced";
  }
  if (value === "policy_enforced") {
    return "policy-enforced";
  }
  return "audit";
}

export function toRolloutCondition(value: unknown): PolicyRolloutCondition {
  const condition = asString(value).toLowerCase();
  if (condition === "applied") {
    return "applied";
  }
  if (condition === "unsupported") {
    return "unsupported";
  }
  return "pending";
}

function toBound(value: unknown): PolicyBound {
  const record = asRecord(value);
  return {
    display: asString(record.display),
    field: asString(record.field),
    label: asString(record.label),
    op: asString(record.op),
    text: asString(record.text),
    value: asNumber(record.value),
  };
}

export function toRenderedRules(value: unknown): readonly PolicyRenderedRule[] {
  return asArray(value).map((item) => {
    const record = asRecord(item);
    return {
      bounds: asArray(record.bounds).map(toBound),
      cedar: asString(record.cedar),
      recognized: asBoolean(record.recognized),
      summary: asArray(record.summary).map(asString),
    };
  });
}

export function toFleetDefaults(value: unknown): PolicyFleetDefaults {
  const record = asRecord(value);
  const defaults: {
    -readonly [Key in keyof PolicyFleetDefaults]: PolicyFleetDefaults[Key];
  } = {};
  const cpuRequest = asOptionalString(record.cpu_request);
  const cpuLimit = asOptionalString(record.cpu_limit);
  const memoryRequest = asOptionalString(record.memory_request);
  const memoryLimit = asOptionalString(record.memory_limit);
  if (cpuRequest) defaults.cpuRequest = cpuRequest;
  if (cpuLimit) defaults.cpuLimit = cpuLimit;
  if (memoryRequest) defaults.memoryRequest = memoryRequest;
  if (memoryLimit) defaults.memoryLimit = memoryLimit;
  return defaults;
}

export function toFindingCounts(value: unknown): PolicyFindingCounts {
  const record = asRecord(value);
  return {
    wouldAddDefaults: asNumber(record.would_add_defaults),
    wouldReject: asNumber(record.would_reject),
  };
}

export function toFinding(value: unknown): PolicyFinding {
  const record = asRecord(value);
  const summary = asRecord(record.request_summary);
  const bundleRevision = asOptionalString(record.bundle_revision);
  return {
    ...(bundleRevision ? { bundleRevision } : {}),
    gatewayId: asString(record.gateway_id),
    id: asString(record.id),
    occurredAt: asString(record.occurred_at),
    operation: asString(record.operation),
    outcome:
      record.outcome === "would_add_defaults"
        ? "would-add-defaults"
        : "would-reject",
    policyNames: asArray(record.policy_names).map(asString),
    principalSubject: asString(record.principal_subject),
    requestSummary: Object.fromEntries(
      Object.entries(summary).filter(
        (entry): entry is [string, number] => typeof entry[1] === "number",
      ),
    ),
  };
}

function toEffectivePolicy(value: unknown): EffectivePolicy {
  const record = asRecord(value);
  const documentationUrl = asOptionalString(record.documentation_url);
  const revision = asOptionalString(record.revision);
  const updatedAt = asOptionalString(record.updated_at);
  return {
    builtIn: asBoolean(record.built_in),
    cedar: asString(record.cedar),
    condition: toRolloutCondition(record.condition),
    ...(documentationUrl ? { documentationUrl } : {}),
    effectiveMode: toPolicyMode(record.effective_mode),
    effectiveModeReason: toPolicyModeReason(record.effective_mode_reason),
    enforcementMode: toPolicyMode(record.enforcement_mode),
    id: asString(record.id),
    name: asString(record.name),
    rendered: toRenderedRules(record.rendered),
    ...(revision ? { revision } : {}),
    ...(updatedAt ? { updatedAt } : {}),
    userMessage: asString(record.user_message),
  };
}

/**
 * Maps `GET /gateways/{id}/effective_policies`. Targets are deliberately not
 * mapped: the API masks them from gateway users (SEC-RBAC-31), and the Policies
 * tab never shows another gateway's identity.
 */
export function toGatewayEffectivePolicies(
  value: unknown,
): GatewayEffectivePolicies {
  const record = asRecord(value);
  return {
    defaults: toFleetDefaults(record.defaults),
    findings: asArray(record.findings).map(toFinding),
    findingsLast24h: toFindingCounts(record.findings_last_24h),
    gatewayId: asString(record.gateway_id),
    globalMode: toPolicyMode(record.global_mode),
    policies: asArray(record.policies).map(toEffectivePolicy),
    schemaVersion: asString(record.schema_version),
  };
}
