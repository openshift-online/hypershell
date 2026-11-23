import type {
  PolicyBound,
  PolicyFleetDefaults,
  PolicyRenderedRule,
  PolicyResourceKey,
} from "../application/policy-types";

/*
 * Kubernetes quantity conversion and rendered-rule attribution shared by the
 * gateway Policies tab and the platform policy editor. The parsing rules match
 * the API server's quantity parser (components/api-server/pkg/cedar) so a value
 * the console converts is the value the server evaluates.
 */

const binarySuffixes: Readonly<Record<string, bigint>> = {
  Ei: 1024n ** 6n,
  Gi: 1024n ** 3n,
  Ki: 1024n,
  Mi: 1024n ** 2n,
  Pi: 1024n ** 5n,
  Ti: 1024n ** 4n,
};

/** Decimal suffixes as a [numerator, denominator] multiplier. */
const decimalSuffixes: Readonly<Record<string, readonly [bigint, bigint]>> = {
  E: [1000n ** 6n, 1n],
  G: [1000n ** 3n, 1n],
  M: [1000n ** 2n, 1n],
  P: [1000n ** 5n, 1n],
  T: [1000n ** 4n, 1n],
  k: [1000n, 1n],
  m: [1n, 1000n],
  n: [1n, 1_000_000_000n],
  u: [1n, 1_000_000n],
};

const numberPattern = /^(\d+)(?:\.(\d+))?$/u;

/** A rational number as numerator / denominator. */
type Rational = readonly [bigint, bigint];

function parseRational(text: string): Rational | undefined {
  const match = numberPattern.exec(text);
  if (!match) {
    return undefined;
  }
  const whole = match[1] ?? "0";
  const fraction = match[2] ?? "";
  const denominator = 10n ** BigInt(fraction.length);
  return [BigInt(whole + fraction), denominator];
}

function parseQuantity(quantity: string): Rational | undefined {
  const text = quantity.trim();
  if (!text) {
    return undefined;
  }
  const binary = binarySuffixes[text.slice(-2)];
  if (binary !== undefined) {
    const value = parseRational(text.slice(0, -2));
    return value ? [value[0] * binary, value[1]] : undefined;
  }
  const decimal = decimalSuffixes[text.slice(-1)];
  if (decimal !== undefined) {
    const value = parseRational(text.slice(0, -1));
    return value ? [value[0] * decimal[0], value[1] * decimal[1]] : undefined;
  }
  return parseRational(text);
}

function wholeNumber([numerator, denominator]: Rational): number | undefined {
  if (numerator % denominator !== 0n) {
    return undefined;
  }
  const value = numerator / denominator;
  return value <= BigInt(Number.MAX_SAFE_INTEGER) ? Number(value) : undefined;
}

/** Converts a CPU quantity (`250m`, `4`, `0.5`) to millicores. */
export function parseCpuMillis(quantity: string): number | undefined {
  const value = parseQuantity(quantity);
  return value ? wholeNumber([value[0] * 1000n, value[1]]) : undefined;
}

/** Converts a memory quantity (`512Mi`, `16Gi`, `1G`) to bytes. */
export function parseMemoryBytes(quantity: string): number | undefined {
  const value = parseQuantity(quantity);
  return value ? wholeNumber(value) : undefined;
}

/** Parses a non-negative whole count, for example a GPU count. */
export function parseCount(quantity: string): number | undefined {
  const text = quantity.trim();
  return /^\d+$/u.test(text) ? Number(text) : undefined;
}

/** Formats millicores the way the API renderer does (`250m`, `4`). */
export function formatCpuMillis(millis: number): string {
  return millis % 1000 === 0 ? String(millis / 1000) : `${String(millis)}m`;
}

/** Formats bytes the way the API renderer does (`512Mi`, `16Gi`). */
export function formatMemoryBytes(bytes: number): string {
  const units = [
    ["Ti", 1024 ** 4],
    ["Gi", 1024 ** 3],
    ["Mi", 1024 ** 2],
    ["Ki", 1024],
  ] as const;
  for (const [suffix, size] of units) {
    if (bytes !== 0 && bytes % size === 0) {
      return `${String(bytes / size)}${suffix}`;
    }
  }
  return String(bytes);
}

/** Context attribute behind each resource row, in display order. */
export const policyResourceFields: readonly (readonly [
  PolicyResourceKey,
  string,
])[] = [
  ["cpuRequest", "cpuRequestMillis"],
  ["cpuLimit", "cpuLimitMillis"],
  ["memoryRequest", "memoryRequestBytes"],
  ["memoryLimit", "memoryLimitBytes"],
  ["gpuCount", "gpuCount"],
];

export function policyResourceForField(
  field: string,
): PolicyResourceKey | undefined {
  return policyResourceFields.find(([, name]) => name === field)?.[0];
}

/** Formats a request-summary value for one context attribute. */
export function formatPolicyFieldValue(field: string, value: number): string {
  const resource = policyResourceForField(field);
  if (resource === "cpuRequest" || resource === "cpuLimit") {
    return formatCpuMillis(value);
  }
  if (resource === "memoryRequest" || resource === "memoryLimit") {
    return formatMemoryBytes(value);
  }
  return String(value);
}

export type PolicyBoundSide = "maximum" | "minimum";

/**
 * Which side of the allowed range a forbid comparison bounds. A forbid matches
 * the rejected region, so `context.x > 4` is a maximum and `context.x < 1` a
 * minimum. Equality comparisons do not bound a range.
 */
export function policyBoundSide(op: string): PolicyBoundSide | undefined {
  if (op === ">" || op === ">=") {
    return "maximum";
  }
  if (op === "<" || op === "<=") {
    return "minimum";
  }
  return undefined;
}

/** A bound that fits a resource row of the limits matrix. */
export interface PolicyRowBound {
  bound: PolicyBound;
  resource: PolicyResourceKey;
  side: PolicyBoundSide;
}

/** The bounds of rendered rules that attribute to a resource row. */
export function rowBounds(
  rules: readonly PolicyRenderedRule[],
): readonly PolicyRowBound[] {
  return rules.flatMap((rule) =>
    rule.recognized
      ? rule.bounds.flatMap((bound) => {
          const resource = policyResourceForField(bound.field);
          const side = policyBoundSide(bound.op);
          return resource && side ? [{ bound, resource, side }] : [];
        })
      : [],
  );
}

/**
 * Rules the limits matrix cannot show as a row bound: statements the renderer
 * did not recognize, and recognized statements with an equality comparison or
 * a field outside the resource rows. These are shown as Cedar with the policy
 * message, never hidden (PLAT-PGP-17).
 */
export function additionalRules(
  rules: readonly PolicyRenderedRule[],
): readonly PolicyRenderedRule[] {
  return rules.filter(
    (rule) =>
      !rule.recognized ||
      rule.bounds.some(
        (bound) =>
          !policyResourceForField(bound.field) || !policyBoundSide(bound.op),
      ),
  );
}

export function fleetDefaultFor(
  defaults: PolicyFleetDefaults,
  resource: PolicyResourceKey,
): string | undefined {
  return resource === "gpuCount" ? undefined : defaults[resource];
}
