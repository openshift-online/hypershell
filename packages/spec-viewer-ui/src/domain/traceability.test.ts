import {
  buildTraceability,
  parseMarker,
  parseMarkerLines,
  testName,
  verificationMethods,
} from "./traceability";

describe("parseMarker", () => {
  it("reads one or more IDs", () => {
    expect(parseMarker("// Verifies: SEC-OIDC-06.01")).toEqual([
      "SEC-OIDC-06.01",
    ]);
    expect(parseMarker("  # Verifies: CPL-OBS-07, CPL-OBS-07.02  ")).toEqual([
      "CPL-OBS-07",
      "CPL-OBS-07.02",
    ]);
  });

  it("rejects malformed markers", () => {
    expect(parseMarker("// Verifies: SEC-OIDC-06.01 and more")).toBeNull();
    expect(parseMarker("//Verifies: SEC-OIDC-06")).toBeNull();
    expect(parseMarker("// Verifies: TS-01")).toBeNull();
    expect(parseMarker("// Verifies: SEC-OIDC-06,SEC-OIDC-07")).toBeNull();
  });
});

describe("testName", () => {
  it.each([
    ["func TestRotate(t *testing.T) {", "TestRotate"],
    ["func (s *Suite) TestRotate() {", "TestRotate"],
    ['\tt.Run("rotates keys", func(t *testing.T) {', "rotates keys"],
    ['it("shows the menu", async () => {', "shows the menu"],
    ["test.each(cases)('handles %s', (c) => {", "handles %s"],
    ["def test_rotate(self):", "test_rotate"],
    ["class RotateTest(unittest.TestCase):", "RotateTest"],
    ["test_rotation() {", "test_rotation"],
    ["  expect(value).toBe(1);", "expect(value).toBe(1);"],
  ])("names %s", (line, name) => {
    expect(testName(["", "// helper comment", line])).toBe(name);
  });

  it("is empty when only comments follow", () => {
    expect(testName(["// one", "  * two", ""])).toBe("");
  });
});

describe("verificationMethods", () => {
  it("assigns a non-Test method to the requirement above it", () => {
    const methods = verificationMethods(`# Spec

### Requirement: ABC-DEF-01 -- Inspected

**Verification:** inspection

#### Scenario: ABC-DEF-01.01 -- Looked at

**Verification:** Analysis

### Requirement: ABC-DEF-02 -- Tested

**Verification:** Test

## Notes

**Verification:** Demonstration

\`\`\`
### Requirement: ABC-DEF-03 -- Example
**Verification:** Inspection
\`\`\`
`);
    expect([...methods]).toEqual([["ABC-DEF-01", "Analysis"]]);
  });

  it("ends a requirement at a malformed requirement ID", () => {
    const methods = verificationMethods(`### Requirement: ABC-DEF-01 -- One
### Requirement: bad-id -- Two
**Verification:** Inspection
`);
    expect(methods.size).toBe(0);
  });

  it("keeps the requirement across malformed headings", () => {
    const methods = verificationMethods(`### Requirement: ABC-DEF-01 -- One
## Requirement: ABC-DEF-09 -- Wrong level
**Verification:** Inspection
`);
    expect([...methods]).toEqual([["ABC-DEF-01", "Inspection"]]);
  });
});

describe("buildTraceability", () => {
  it("only reads methods from .spec.md files", () => {
    const traced = buildTraceability(
      [
        {
          path: "notes.md",
          content:
            "### Requirement: ABC-DEF-01 -- One\n**Verification:** Inspection\n",
        },
      ],
      [],
    );
    expect(traced.size).toBe(0);
  });
});

describe("parseMarkerLines", () => {
  it("accepts the server response", () => {
    const line = { path: "a_test.go", line: 2, text: "x", following: ["y"] };
    expect(parseMarkerLines([line])).toEqual([line]);
  });

  it("rejects other shapes", () => {
    expect(() => parseMarkerLines({})).toThrow("not a list");
    expect(() =>
      parseMarkerLines([{ path: "a", line: 1, text: "x", following: [1] }]),
    ).toThrow("malformed entry");
  });
});
