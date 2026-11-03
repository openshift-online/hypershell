import { markedMarkdown } from "../adapters/marked-markdown";
import {
  buildConformanceReport,
  checkScenario,
  conformancePercent,
  emptyCounts,
} from "./conformance";
import { buildCatalog, parseSpecFile } from "./spec-document";
import { buildTraceability } from "./traceability";

const GOOD = `# Good

### Requirement: GD-01 -- Rotates

The platform SHALL rotate.

#### Scenario: GD-01-S1 -- Rotation

- GIVEN a secret
- WHEN it expires
- THEN it SHALL rotate
`;

const MIXED = `# Mixed

### Requirement: Untagged

The controller MUST retry.

#### Scenario: Retry

- **WHEN** a call fails
- **THEN** it retries

#### Scenario: Backwards

- THEN something happens
- WHEN something else

### Requirement: MX-02 -- Lonely

Nothing normative here.

### Requirement: MX-03 -- Prose

The system SHALL work.

#### Scenario: MX-03-S1: Prose only

When things happen, then it works.
`;

function report() {
  return buildConformanceReport(
    buildCatalog(
      [
        ["platform/good.spec.md", GOOD],
        ["platform/sub/mixed.spec.md", MIXED],
        ["web/persona.md", "# Persona\n\nNo requirements.\n"],
      ].map(([path = "", content = ""]) =>
        parseSpecFile(path, markedMarkdown.lex(content)),
      ),
    ),
  );
}

describe("checkScenario", () => {
  it("accepts WHEN then THEN, with optional GIVEN, AND, and bold keywords", () => {
    expect(checkScenario("- GIVEN a\n- AND b\n- WHEN c\n- THEN d")).toEqual([]);
    expect(checkScenario("1. **WHEN** c\n2. __THEN__ d")).toEqual([]);
  });

  it("reports missing and out-of-order steps", () => {
    expect(checkScenario("- GIVEN a\n- THEN d")).toEqual(["missing-when"]);
    expect(checkScenario("When x, then y.")).toEqual([
      "missing-when",
      "missing-then",
    ]);
    expect(checkScenario("- THEN d\n- WHEN c")).toEqual(["then-before-when"]);
  });
});

describe("buildConformanceReport", () => {
  it("checks each requirement and lists its issues", () => {
    const { root } = report();
    const mixed = root.folders[0]?.folders[0]?.files[0];
    expect(mixed?.requirements.map((r) => [r.node.title, r.issues])).toEqual([
      [
        "Untagged",
        ["malformed-scenarios", "missing-id", "scenarios-missing-id"],
      ],
      ["Lonely", ["no-scenarios", "no-normative"]],
      ["Prose", ["malformed-scenarios"]],
    ]);
    expect(
      mixed?.requirements[0]?.scenarios.map((s) => [s.node.title, s.issues]),
    ).toEqual([
      ["Retry", ["missing-id"]],
      ["Backwards", ["then-before-when", "missing-id"]],
    ]);
    expect(
      mixed?.requirements[2]?.scenarios.map((s) => [s.node.specId, s.issues]),
    ).toEqual([["MX-03-S1", ["missing-when", "missing-then"]]]);
  });

  it("sums counts up the folder tree", () => {
    const { root, filesWithoutRequirements } = report();
    expect(root.folders.map((folder) => folder.path)).toEqual(["platform"]);
    expect(root.folders[0]?.folders.map((folder) => folder.path)).toEqual([
      "platform/sub",
    ]);
    expect(root.counts).toEqual({
      requirements: 4,
      conformant: 1,
      withScenarios: 3,
      scenarios: 4,
      wellFormedScenarios: 2,
      missingId: 1,
      scenariosMissingId: 2,
      testedScenarios: 0,
      untestedScenarios: 0,
    });
    expect(conformancePercent(root.counts)).toBe(25);
    expect(filesWithoutRequirements.map((file) => file.path)).toEqual([
      "web/persona.md",
    ]);
  });

  it("has no percentage without requirements", () => {
    expect(conformancePercent(emptyCounts())).toBeNull();
  });
});

describe("test traceability", () => {
  const catalog = () =>
    buildCatalog(
      [
        ["platform/good.spec.md", GOOD],
        [
          "platform/tested.spec.md",
          `# Tested

### Requirement: TST-ANS-01 -- Two scenarios

The service SHALL answer.

#### Scenario: TST-ANS-01.01 -- Cited

- WHEN asked
- THEN it answers

#### Scenario: TST-ANS-01.02 -- Not cited

- WHEN asked twice
- THEN it answers twice

### Requirement: TST-ANS-02 -- Inspected

The design SHALL be reviewed.

**Verification:** Inspection

#### Scenario: TST-ANS-02.01 -- Reviewed

- WHEN reviewed
- THEN approved
`,
        ],
      ].map(([path = "", content = ""]) =>
        parseSpecFile(path, markedMarkdown.lex(content)),
      ),
    );
  const traceability = buildTraceability(catalog().files, [
    {
      path: "app/answer.test.ts",
      line: 4,
      text: "// Verifies: TST-ANS-01.01",
      following: ["", "// Answers.", 'it("answers", () => {'],
    },
    {
      path: "app/bad.test.ts",
      line: 1,
      text: "// Verifies: TST-ANS-01.02 extra",
      following: ["it('ignored')"],
    },
  ]);

  it("builds citations and verification methods from markers", () => {
    expect(traceability.get("TST-ANS-01.01")?.citations).toEqual([
      { path: "app/answer.test.ts", line: 4, test: "answers" },
    ]);
    expect(traceability.get("TST-ANS-01.02")).toBeUndefined();
    expect(traceability.get("TST-ANS-02")?.method).toBe("Inspection");
  });

  it("flags scenarios no test cites, except under non-Test requirements", () => {
    const tested = buildConformanceReport(
      catalog(),
      traceability,
    ).root.folders[0]?.files.find((file) =>
      file.file.path.endsWith("tested.spec.md"),
    );
    expect(
      tested?.requirements.map((r) => [
        r.node.specId,
        r.issues,
        r.nonTestMethod,
        r.scenarios.map((s) => [s.node.specId, s.issues, s.citations.length]),
      ]),
    ).toEqual([
      [
        "TST-ANS-01",
        ["untested-scenarios"],
        undefined,
        [
          ["TST-ANS-01.01", [], 1],
          ["TST-ANS-01.02", ["untested"], 0],
        ],
      ],
      ["TST-ANS-02", [], "Inspection", [["TST-ANS-02.01", [], 0]]],
    ]);
    expect(tested?.counts).toMatchObject({
      conformant: 1,
      testedScenarios: 1,
      untestedScenarios: 1,
    });
  });

  it("leaves the test check out when the report is unavailable", () => {
    const { root } = buildConformanceReport(catalog());
    expect(root.counts).toMatchObject({
      testedScenarios: 0,
      untestedScenarios: 0,
      conformant: 3,
    });
  });
});
