import { describe, expect, it } from "vitest";

import { tokenizeCedar } from "./cedar-highlight";

describe("tokenizeCedar", () => {
  // Verifies: WEB-PADM-05
  it("classifies Cedar tokens and keeps the source intact", () => {
    const source = [
      '@id("cpu-cap")',
      "// at most 4 CPUs",
      'forbid (principal, action == OpenShell::Action::"CreateSandbox", resource)',
      "when { context.cpuRequestMillis > 4000 && true };",
    ].join("\n");

    const tokens = tokenizeCedar(source);

    expect(tokens.map(({ value }) => value).join("")).toBe(source);
    const kindOf = (value: string) =>
      tokens.find((token) => token.value === value)?.kind;
    expect(kindOf("@id")).toBe("annotation");
    expect(kindOf('"cpu-cap"')).toBe("string");
    expect(kindOf("// at most 4 CPUs")).toBe("comment");
    expect(kindOf("forbid")).toBe("keyword");
    expect(kindOf("when")).toBe("keyword");
    expect(kindOf("true")).toBe("keyword");
    expect(kindOf("principal")).toBe("variable");
    expect(kindOf("context")).toBe("variable");
    expect(kindOf("OpenShell")).toBe("type");
    expect(kindOf("4000")).toBe("number");
    expect(kindOf("&&")).toBe("operator");
  });

  it("tolerates an unterminated string", () => {
    const tokens = tokenizeCedar('"open');
    expect(tokens).toEqual([{ kind: "string", value: '"open' }]);
  });
});
