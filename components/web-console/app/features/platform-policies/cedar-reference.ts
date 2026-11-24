/*
 * Reference data for the Cedar editor's "What you can use" panel
 * (WEB-PADM-07). It mirrors the v1 HyperShell Cedar schema
 * (components/api-server/pkg/cedar/schema.go, PLAT-PGP-03): keep it in step
 * with the schema version the policy bundle carries.
 */

export type CedarReferenceUnit =
  "bytes" | "gpus" | "imageReference" | "millicores" | "sandboxName";

export interface CedarContextAttribute {
  example: string;
  name: string;
  type: "Long" | "String";
  unit: CedarReferenceUnit;
}

export const cedarContextAttributes: readonly CedarContextAttribute[] = [
  {
    example: "500m = 500",
    name: "cpuRequestMillis",
    type: "Long",
    unit: "millicores",
  },
  {
    example: "2 = 2000",
    name: "cpuLimitMillis",
    type: "Long",
    unit: "millicores",
  },
  {
    example: "1Gi = 1073741824",
    name: "memoryRequestBytes",
    type: "Long",
    unit: "bytes",
  },
  {
    example: "4Gi = 4294967296",
    name: "memoryLimitBytes",
    type: "Long",
    unit: "bytes",
  },
  { example: "1", name: "gpuCount", type: "Long", unit: "gpus" },
  {
    example: '"quay.io/org/img:1"',
    name: "image",
    type: "String",
    unit: "imageReference",
  },
  {
    example: '"my-sandbox"',
    name: "sandboxName",
    type: "String",
    unit: "sandboxName",
  },
];

export type CedarEntityAttributeType = "setOfString" | "string";

export interface CedarEntityAttribute {
  name: string;
  /** Gateway name and id attributes carry a clarifying note. */
  note?: "gatewayId" | "gatewayName";
  type: CedarEntityAttributeType;
}

export const cedarEntityAttributes: readonly CedarEntityAttribute[] = [
  { name: "principal.subject", type: "string" },
  { name: "principal.roles", type: "setOfString" },
  { name: "resource.id", note: "gatewayId", type: "string" },
  { name: "resource.name", note: "gatewayName", type: "string" },
];

/** Actions: available in v1, or planned for a later schema version. */
export const cedarAvailableActions: readonly string[] = ["CreateSandbox"];

export const cedarLaterActions: readonly string[] = [
  "ResumeSandbox",
  "UpdateSandboxPolicy",
];

export const cedarOperators: readonly string[] = [
  "==",
  "!=",
  "<",
  "<=",
  ">",
  ">=",
  "&&",
  "||",
  "!",
  "like",
  ".contains()",
  ".containsAny()",
  "in",
  "has",
];

export type CedarUnavailableConstruct =
  "annotations" | "ip" | "permit" | "quotas" | "templates" | "time";

export const cedarUnavailableConstructs: readonly CedarUnavailableConstruct[] =
  ["permit", "templates", "quotas", "time", "ip", "annotations"];

/** Code tokens shown inside reference sentences. */
export const cedarTokens = {
  annotation: "@",
  forbid: "forbid",
  id: "@id",
  ip: "ip()",
  permit: "permit",
} as const;
