import { describe, expect, it } from "vitest";

import {
  appendCedar,
  buildCedarSnippet,
  cedarSnippet,
  cedarStatementLines,
  splitList,
} from "./cedar-snippets";

function cedarOf(result: ReturnType<typeof buildCedarSnippet>): string {
  if (!result.ok) {
    throw new Error(`expected cedar, got ${JSON.stringify(result.errors)}`);
  }
  return result.cedar;
}

describe("cedar snippets", () => {
  // Verifies: WEB-PADM-06
  it("converts a CPU request range in quantities to millicores", () => {
    const cedar = cedarOf(
      buildCedarSnippet("cpu-request-range", { maximum: "4", minimum: "250m" }),
    );
    expect(cedar).toContain(
      "context.cpuRequestMillis < 250 || context.cpuRequestMillis > 4000",
    );
    expect(cedar).toContain('action == OpenShell::Action::"CreateSandbox"');
    expect(cedar).not.toContain("ResumeSandbox");
    expect(cedar.startsWith("forbid (")).toBe(true);
  });

  // Verifies: WEB-PADM-06
  it("emits renderer-recognized bounds for every resource snippet", () => {
    expect(
      cedarOf(buildCedarSnippet("cpu-limit-max", { maximum: "8" })),
    ).toContain("when { context.cpuLimitMillis > 8000 };");
    expect(
      cedarOf(
        buildCedarSnippet("memory-request-range", {
          maximum: "16Gi",
          minimum: "512Mi",
        }),
      ),
    ).toContain(
      "context.memoryRequestBytes < 536870912 || context.memoryRequestBytes > 17179869184",
    );
    expect(
      cedarOf(buildCedarSnippet("memory-limit-max", { maximum: "32Gi" })),
    ).toContain("context.memoryLimitBytes > 34359738368");
    expect(cedarOf(buildCedarSnippet("gpu-max", { maximum: "1" }))).toContain(
      "context.gpuCount > 1",
    );
    expect(
      cedarOf(buildCedarSnippet("cpu-request-range", { minimum: "0.5" })),
    ).toContain("when { context.cpuRequestMillis < 500 };");
  });

  // Verifies: WEB-PADM-06
  it("builds image registry and GPU role rules", () => {
    expect(
      cedarOf(
        buildCedarSnippet("image-registries", {
          registries: "quay.io/my-org/, registry.redhat.io",
        }),
      ),
    ).toContain(
      'when { !(context.image like "quay.io/my-org/*" || context.image like "registry.redhat.io/*") };',
    );
    expect(
      cedarOf(buildCedarSnippet("gpu-roles", { roles: "gpu-users" })),
    ).toContain(
      'when { context.gpuCount > 0 }\nunless { principal.roles.contains("gpu-users") };',
    );
    expect(
      cedarOf(buildCedarSnippet("gpu-roles", { roles: 'a, b"c' })),
    ).toContain('principal.roles.containsAny(["a", "b\\"c"])');
  });

  // Verifies: WEB-PADM-06
  it("reports invalid quantities, missing values, and inverted ranges", () => {
    expect(buildCedarSnippet("cpu-request-range", { minimum: "lots" })).toEqual(
      { errors: { minimum: "invalid-cpu" }, ok: false },
    );
    expect(buildCedarSnippet("memory-limit-max", { maximum: "1.5" })).toEqual({
      errors: { maximum: "invalid-memory" },
      ok: false,
    });
    expect(buildCedarSnippet("gpu-max", { maximum: "1.5" })).toEqual({
      errors: { maximum: "invalid-count" },
      ok: false,
    });
    expect(buildCedarSnippet("cpu-request-range", {})).toEqual({
      errors: { minimum: "missing-value" },
      ok: false,
    });
    expect(
      buildCedarSnippet("cpu-request-range", { maximum: "1", minimum: "2" }),
    ).toEqual({ errors: { minimum: "minimum-above-maximum" }, ok: false });
    expect(buildCedarSnippet("image-registries", {})).toEqual({
      errors: { registries: "missing-value" },
      ok: false,
    });
    expect(buildCedarSnippet("gpu-roles", { roles: " , " })).toEqual({
      errors: { roles: "missing-value" },
      ok: false,
    });
  });

  it("appends statements with one blank line between them", () => {
    expect(appendCedar("", "forbid x;")).toBe("forbid x;\n");
    expect(appendCedar("forbid a;\n\n\n", "forbid b;")).toBe(
      "forbid a;\n\nforbid b;\n",
    );
  });

  it("finds the line where each statement starts", () => {
    const cedar = [
      "// comment; with a semicolon",
      "forbid (principal, action, resource)",
      'when { context.image like "a;b" };',
      "",
      "permit (principal, action, resource);",
    ].join("\n");
    expect(cedarStatementLines(cedar)).toEqual([2, 5]);
    expect(cedarStatementLines('forbid "x\\"\ny";\nforbid z;')).toEqual([1, 3]);
  });

  it("splits lists and rejects unknown snippets", () => {
    expect(splitList("a,\nb, ,c")).toEqual(["a", "b", "c"]);
    expect(() => cedarSnippet("nope" as never)).toThrow(
      "unknown cedar snippet",
    );
  });
});
