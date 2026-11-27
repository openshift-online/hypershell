import {
  parseCount,
  parseCpuMillis,
  parseMemoryBytes,
  policyResourceFields,
  rowBounds,
  type PolicyRenderedRule,
  type PolicyResourceKey,
} from "@openshift-online/hypershell-gateway-management-ui";

import type {
  PlatformPolicy,
  PlatformPolicyDraft,
  PlatformPolicyEvaluation,
  PlatformPolicyTestRequest,
} from "../../application/platform-policies/platform-policy-types";
import { routePaths } from "../../../shared/routes";
import { cedarStatementLines, splitList } from "./cedar-snippets";

/*
 * Pure editor model for the platform policy form (WEB-PADM-05..09): form
 * values, the conversion to an application draft and a test request, and the
 * preview and test-result projections of an evaluation. Views stay free of
 * data shaping.
 */

export type PlatformPolicyTargetScope = "all" | "all-except" | "selected";

export interface PlatformPolicyFormValues {
  cedar: string;
  description: string;
  documentationUrl: string;
  name: string;
  targetGateways: readonly string[];
  targetScope: PlatformPolicyTargetScope;
  userMessage: string;
}

export interface PlatformPolicyTestValues {
  cpuLimit: string;
  cpuRequest: string;
  gpuCount: string;
  image: string;
  memoryLimit: string;
  memoryRequest: string;
  roles: string;
}

export type PlatformPolicyTestField = keyof PlatformPolicyTestValues;

export type PlatformPolicyTestError =
  "invalid-count" | "invalid-cpu" | "invalid-memory";

export const emptyPlatformPolicyFormValues: PlatformPolicyFormValues =
  Object.freeze({
    cedar: "",
    description: "",
    documentationUrl: "",
    name: "",
    targetGateways: Object.freeze([]),
    targetScope: "all",
    userMessage: "",
  });

export const emptyPlatformPolicyTestValues: PlatformPolicyTestValues =
  Object.freeze({
    cpuLimit: "",
    cpuRequest: "",
    gpuCount: "",
    image: "",
    memoryLimit: "",
    memoryRequest: "",
    roles: "",
  });

export function toPlatformPolicyDraft(
  values: PlatformPolicyFormValues,
): PlatformPolicyDraft {
  const gateways = [...values.targetGateways];
  return {
    cedar: values.cedar,
    description: values.description.trim(),
    documentationUrl: values.documentationUrl.trim(),
    name: values.name.trim(),
    target:
      values.targetScope === "selected"
        ? { all: false, exclude: [], gatewayIds: gateways }
        : {
            all: true,
            exclude: values.targetScope === "all-except" ? gateways : [],
            gatewayIds: [],
          },
    userMessage: values.userMessage.trim(),
  };
}

export function platformPolicyFormValues(
  policy: PlatformPolicy,
): PlatformPolicyFormValues {
  const { target } = policy;
  let targetScope: PlatformPolicyTargetScope = "all";
  let targetGateways: readonly string[] = [];
  if (!target.all) {
    targetScope = "selected";
    targetGateways = target.gatewayIds;
  } else if (target.exclude.length) {
    targetScope = "all-except";
    targetGateways = target.exclude;
  }
  return {
    cedar: policy.cedar,
    description: policy.description ?? "",
    documentationUrl: policy.documentationUrl ?? "",
    name: policy.name,
    targetGateways,
    targetScope,
    userMessage: policy.userMessage,
  };
}

/**
 * Converts the test form to a test request. Empty fields are omitted so the
 * server applies the fleet defaults; invalid quantities are reported per field
 * and omitted, because the server rejects the whole dry run for a malformed
 * quantity.
 */
export function toPlatformPolicyTestRequest(values: PlatformPolicyTestValues): {
  errors: Partial<Record<PlatformPolicyTestField, PlatformPolicyTestError>>;
  request: PlatformPolicyTestRequest;
} {
  const errors: Partial<
    Record<PlatformPolicyTestField, PlatformPolicyTestError>
  > = {};
  const request: {
    -readonly [
      Key in keyof PlatformPolicyTestRequest
    ]: PlatformPolicyTestRequest[Key];
  } = {};
  const quantity = (
    field: "cpuLimit" | "cpuRequest" | "memoryLimit" | "memoryRequest",
  ) => {
    const raw = values[field].trim();
    if (!raw) {
      return;
    }
    const isCpu = field === "cpuLimit" || field === "cpuRequest";
    const parsed = isCpu ? parseCpuMillis(raw) : parseMemoryBytes(raw);
    if (parsed === undefined) {
      errors[field] = isCpu ? "invalid-cpu" : "invalid-memory";
    } else {
      request[field] = raw;
    }
  };
  quantity("cpuRequest");
  quantity("cpuLimit");
  quantity("memoryRequest");
  quantity("memoryLimit");
  const gpus = values.gpuCount.trim();
  if (gpus) {
    const parsed = parseCount(gpus);
    if (parsed === undefined) {
      errors.gpuCount = "invalid-count";
    } else {
      request.gpuCount = parsed;
    }
  }
  if (values.image.trim()) {
    request.image = values.image.trim();
  }
  const roles = splitList(values.roles);
  if (roles.length) {
    request.roles = roles;
  }
  return { errors, request };
}

export type PlatformPolicyPreviewRow =
  | {
      kind: "resource";
      maximum?: string;
      minimum?: string;
      resource: PolicyResourceKey;
    }
  | { kind: "additional"; line?: number; statement: number };

/** What the gateway Policies tab would show for these rendered rules. */
export function platformPolicyPreviewRows(
  rules: readonly PolicyRenderedRule[],
  cedar: string,
): readonly PlatformPolicyPreviewRow[] {
  const bounds = rowBounds(rules);
  const lines = cedarStatementLines(cedar);
  const resourceRows = policyResourceFields.flatMap(
    ([resource]): PlatformPolicyPreviewRow[] => {
      const forResource = bounds.filter((bound) => bound.resource === resource);
      if (!forResource.length) {
        return [];
      }
      const pick = (side: "maximum" | "minimum") => {
        const values = forResource
          .filter((bound) => bound.side === side)
          .map(({ bound }) => bound.display);
        return values.length ? values.join(", ") : undefined;
      };
      const minimum = pick("minimum");
      const maximum = pick("maximum");
      return [
        {
          kind: "resource",
          resource,
          ...(minimum === undefined ? {} : { minimum }),
          ...(maximum === undefined ? {} : { maximum }),
        },
      ];
    },
  );
  const additional = rules.flatMap(
    (rule, statement): PlatformPolicyPreviewRow[] => {
      const fullyBounded =
        rule.recognized &&
        rule.bounds.length > 0 &&
        rowBounds([rule]).length === rule.bounds.length;
      if (fullyBounded) {
        return [];
      }
      const line = lines[statement];
      return [
        {
          kind: "additional",
          statement,
          ...(line === undefined ? {} : { line }),
        },
      ];
    },
  );
  return [...resourceRows, ...additional];
}

export interface PlatformPolicyRejection {
  line?: number;
  rule?: PolicyRenderedRule;
  statement: number;
}

/** The statements that reject the test request, with their editor lines. */
export function platformPolicyRejections(
  evaluation: PlatformPolicyEvaluation,
  cedar: string,
): readonly PlatformPolicyRejection[] {
  const lines = cedarStatementLines(cedar);
  return evaluation.rejectingStatements.map((statement) => {
    const line = lines[statement];
    const rule = evaluation.rendered[statement];
    return {
      statement,
      ...(line === undefined ? {} : { line }),
      ...(rule === undefined ? {} : { rule }),
    };
  });
}

/** The first non-comment line of a statement, for a compact reference. */
export function cedarStatementHeadline(rule: PolicyRenderedRule): string {
  return (
    rule.cedar
      .split("\n")
      .map((line) => line.trim())
      .find(
        (line) => line && !line.startsWith("//") && !line.startsWith("@"),
      ) ?? ""
  );
}

// Attributes of the CPU and memory rules the Add a rule snippets insert
// (WEB-PADM-06).
const resourceRuleAttribute =
  /\b(?:cpuLimitMillis|cpuRequestMillis|memoryLimitBytes|memoryRequestBytes)\b/u;

/** Whether the draft Cedar contains a CPU or memory rule. */
export function hasResourceLimitRule(cedar: string): boolean {
  return resourceRuleAttribute.test(cedar);
}

/**
 * The console resource limit help page for a policy (WEB-PADM-13,
 * WEB-UDOC-09), with the policy name URL-encoded.
 */
export function resourceLimitHelpUrl(
  origin: string,
  policyName: string,
): string {
  const name = policyName.trim();
  const page = `${origin}${routePaths.docsRefusalResourceLimits}`;
  return name === "" ? page : `${page}?policy=${encodeURIComponent(name)}`;
}

/**
 * The editor offers the help page only for a named draft with a CPU or memory
 * rule and an empty documentation URL, so it never replaces an entered value
 * and the link always carries the policy name.
 */
export function shouldSuggestResourceLimitHelp(
  values: Pick<PlatformPolicyFormValues, "cedar" | "documentationUrl" | "name">,
): boolean {
  return (
    values.name.trim() !== "" &&
    values.documentationUrl.trim() === "" &&
    hasResourceLimitRule(values.cedar)
  );
}
