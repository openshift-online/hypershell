import {
  parseCount,
  parseCpuMillis,
  parseMemoryBytes,
} from "@openshift-online/hypershell-gateway-management-ui";

/*
 * Add-a-rule snippets for the Cedar policy editor (WEB-PADM-06). Each snippet
 * accepts Kubernetes quantities, converts them to the schema units
 * (PLAT-PGP-03: millicores, bytes, counts), and emits Cedar the plain-words
 * renderer recognizes (PLAT-PGP-17) for the resource snippets.
 *
 * The v1 HyperShell Cedar schema models only the CreateSandbox action, so every
 * snippet scopes its statement to `OpenShell::Action::"CreateSandbox"`. A
 * statement naming an action the schema does not declare fails write-time
 * validation (PLAT-PGP-04).
 */

export type CedarSnippetId =
  | "cpu-limit-max"
  | "cpu-request-range"
  | "gpu-max"
  | "gpu-roles"
  | "image-registries"
  | "memory-limit-max"
  | "memory-request-range";

export type CedarSnippetField = "maximum" | "minimum" | "registries" | "roles";

export type CedarSnippetValueKind = "count" | "cpu" | "list" | "memory";

export interface CedarSnippetDefinition {
  /** Context attribute the snippet bounds, when it bounds one. */
  attribute?: string;
  fields: readonly CedarSnippetField[];
  id: CedarSnippetId;
  /** Whether at least one of the listed fields must be filled. */
  kind: CedarSnippetValueKind;
}

export const cedarSnippets: readonly CedarSnippetDefinition[] = [
  {
    attribute: "cpuRequestMillis",
    fields: ["minimum", "maximum"],
    id: "cpu-request-range",
    kind: "cpu",
  },
  {
    attribute: "cpuLimitMillis",
    fields: ["maximum"],
    id: "cpu-limit-max",
    kind: "cpu",
  },
  {
    attribute: "memoryRequestBytes",
    fields: ["minimum", "maximum"],
    id: "memory-request-range",
    kind: "memory",
  },
  {
    attribute: "memoryLimitBytes",
    fields: ["maximum"],
    id: "memory-limit-max",
    kind: "memory",
  },
  { attribute: "gpuCount", fields: ["maximum"], id: "gpu-max", kind: "count" },
  { fields: ["registries"], id: "image-registries", kind: "list" },
  { fields: ["roles"], id: "gpu-roles", kind: "list" },
];

/** The only action the v1 schema models. */
export const cedarCreateSandboxAction = 'OpenShell::Action::"CreateSandbox"';

/** Operation names shown in the snippet form. */
export const cedarSnippetOperations: readonly string[] = ["CreateSandbox"];

export type CedarSnippetError =
  | "invalid-count"
  | "invalid-cpu"
  | "invalid-memory"
  | "minimum-above-maximum"
  | "missing-value";

export type CedarSnippetValues = Partial<Record<CedarSnippetField, string>>;

export type CedarSnippetResult =
  | { cedar: string; ok: true }
  | {
      errors: Partial<Record<CedarSnippetField, CedarSnippetError>>;
      ok: false;
    };

export function cedarSnippet(id: CedarSnippetId): CedarSnippetDefinition {
  const definition = cedarSnippets.find((snippet) => snippet.id === id);
  if (!definition) {
    throw new Error(`unknown cedar snippet: ${id}`);
  }
  return definition;
}

function statement(condition: string, unless?: string): string {
  return [
    "forbid (",
    "  principal,",
    `  action == ${cedarCreateSandboxAction},`,
    "  resource",
    ")",
    unless === undefined
      ? `when { ${condition} };`
      : `when { ${condition} }\nunless { ${unless} };`,
  ].join("\n");
}

/** Splits a comma- or newline-separated list, dropping blanks. */
export function splitList(value: string | undefined): readonly string[] {
  return (value ?? "")
    .split(/[,\n]/u)
    .map((item) => item.trim())
    .filter(Boolean);
}

function cedarString(value: string): string {
  return `"${value.replaceAll("\\", "\\\\").replaceAll('"', '\\"')}"`;
}

/** Turns a registry or registry prefix into a Cedar `like` pattern. */
function registryPattern(registry: string): string {
  const trimmed = registry.replace(/\*+$/u, "").replace(/\/+$/u, "");
  // `like` treats `*` as a wildcard; escape any literal asterisk the author
  // typed inside the registry name.
  return cedarString(`${trimmed.replaceAll("*", "\\*")}/*`);
}

function parseFor(kind: CedarSnippetValueKind) {
  if (kind === "cpu") {
    return { error: "invalid-cpu" as const, parse: parseCpuMillis };
  }
  if (kind === "memory") {
    return { error: "invalid-memory" as const, parse: parseMemoryBytes };
  }
  return { error: "invalid-count" as const, parse: parseCount };
}

function buildRange(
  definition: CedarSnippetDefinition,
  values: CedarSnippetValues,
): CedarSnippetResult {
  const { error, parse } = parseFor(definition.kind);
  const errors: Partial<Record<CedarSnippetField, CedarSnippetError>> = {};
  const bounds: Partial<Record<"maximum" | "minimum", number>> = {};
  for (const field of definition.fields) {
    if (field !== "minimum" && field !== "maximum") {
      continue;
    }
    const raw = values[field]?.trim() ?? "";
    if (!raw) {
      continue;
    }
    const parsed = parse(raw);
    if (parsed === undefined) {
      errors[field] = error;
    } else {
      bounds[field] = parsed;
    }
  }
  if (Object.keys(errors).length) {
    return { errors, ok: false };
  }
  const { maximum, minimum } = bounds;
  if (minimum === undefined && maximum === undefined) {
    const first = definition.fields[0] ?? "maximum";
    return { errors: { [first]: "missing-value" }, ok: false };
  }
  if (minimum !== undefined && maximum !== undefined && minimum > maximum) {
    return { errors: { minimum: "minimum-above-maximum" }, ok: false };
  }
  const attribute = `context.${definition.attribute ?? ""}`;
  const clauses = [
    ...(minimum === undefined ? [] : [`${attribute} < ${String(minimum)}`]),
    ...(maximum === undefined ? [] : [`${attribute} > ${String(maximum)}`]),
  ];
  return { cedar: statement(clauses.join(" || ")), ok: true };
}

/**
 * Builds the Cedar statement for a snippet, or field errors when a value is
 * missing or is not a valid Kubernetes quantity.
 */
export function buildCedarSnippet(
  id: CedarSnippetId,
  values: CedarSnippetValues,
): CedarSnippetResult {
  const definition = cedarSnippet(id);
  if (id === "image-registries") {
    const registries = splitList(values.registries);
    if (!registries.length) {
      return { errors: { registries: "missing-value" }, ok: false };
    }
    const allowed = registries
      .map((registry) => `context.image like ${registryPattern(registry)}`)
      .join(" || ");
    return { cedar: statement(`!(${allowed})`), ok: true };
  }
  if (id === "gpu-roles") {
    const roles = splitList(values.roles);
    if (!roles.length) {
      return { errors: { roles: "missing-value" }, ok: false };
    }
    const unless =
      roles.length === 1
        ? `principal.roles.contains(${cedarString(roles[0] ?? "")})`
        : `principal.roles.containsAny([${roles.map(cedarString).join(", ")}])`;
    return { cedar: statement("context.gpuCount > 0", unless), ok: true };
  }
  return buildRange(definition, values);
}

/** Appends a statement to the draft with one blank line between them. */
export function appendCedar(existing: string, addition: string): string {
  const trimmed = existing.replace(/\s+$/u, "");
  return trimmed ? `${trimmed}\n\n${addition}\n` : `${addition}\n`;
}

/**
 * Returns the 1-based line where each statement begins, in statement order, so
 * statement indexes from the evaluator (rejecting statements) can be shown as
 * editor lines. Comments and string literals are skipped; a statement ends at
 * a top-level `;`.
 */
export function cedarStatementLines(cedar: string): readonly number[] {
  const lines: number[] = [];
  let line = 1;
  let inStatement = false;
  for (let index = 0; index < cedar.length; index += 1) {
    const character = cedar.charAt(index);
    if (character === "\n") {
      line += 1;
      continue;
    }
    if (character === "/" && cedar.charAt(index + 1) === "/") {
      while (index + 1 < cedar.length && cedar.charAt(index + 1) !== "\n") {
        index += 1;
      }
      continue;
    }
    if (/\s/u.test(character)) {
      continue;
    }
    if (!inStatement) {
      inStatement = true;
      lines.push(line);
    }
    if (character === '"') {
      index += 1;
      while (index < cedar.length && cedar.charAt(index) !== '"') {
        if (cedar.charAt(index) === "\\") {
          index += 1;
        } else if (cedar.charAt(index) === "\n") {
          line += 1;
        }
        index += 1;
      }
      continue;
    }
    if (character === ";") {
      inStatement = false;
    }
  }
  return lines;
}
