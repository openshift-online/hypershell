// Spec conformance: how many requirements in each file follow the spec
// format. A requirement conforms when:
//
//   1. it has at least one scenario,
//   2. every scenario has a WHEN step followed by a THEN step,
//   3. its own text uses a normative keyword (SHALL or MUST), and
//   4. its heading carries a requirement ID such as ABC-DEF-01, and
//   5. every scenario heading carries a scenario ID such as ABC-DEF-01.01, and
//   6. every scenario is cited by a test marker (`// Verifies: ABC-DEF-01.01`),
//      unless the requirement is verified another way (inspection,
//      analysis, or demonstration). This check runs only when the
//      traceability report is available.
//
// Steps are list items or lines that start with an uppercase keyword:
// GIVEN, WHEN, THEN, AND, or BUT, optionally in bold.

import type { SpecCatalog, SpecFile, SpecNode } from "./spec-document";
import {
  isTestVerified,
  type TestCitation,
  type Traceability,
} from "./traceability";

export type ScenarioIssue =
  | "missing-when"
  | "missing-then"
  | "then-before-when"
  | "missing-id"
  | "untested";

export type RequirementIssue =
  | "no-scenarios"
  | "malformed-scenarios"
  | "no-normative"
  | "missing-id"
  | "scenarios-missing-id"
  | "untested-scenarios";

export interface ScenarioReport {
  node: SpecNode;
  issues: ScenarioIssue[];
  /** Tests whose `Verifies:` marker cites this scenario. */
  citations: TestCitation[];
}

export interface RequirementReport {
  node: SpecNode;
  scenarios: ScenarioReport[];
  issues: RequirementIssue[];
  /** Verification method when it is not "Test", such as "Inspection". */
  nonTestMethod?: string;
}

export interface ConformanceCounts {
  requirements: number;
  /** Requirements that pass every check. */
  conformant: number;
  /** Requirements with at least one scenario. */
  withScenarios: number;
  scenarios: number;
  /** Scenarios with a WHEN step followed by a THEN step. */
  wellFormedScenarios: number;
  /** Requirements without an ID. */
  missingId: number;
  /** Scenarios without an ID. */
  scenariosMissingId: number;
  /** Scenarios a test marker cites. */
  testedScenarios: number;
  /** Scenarios of test-verified requirements that no test marker cites. */
  untestedScenarios: number;
}

export interface FileReport {
  file: SpecFile;
  requirements: RequirementReport[];
  counts: ConformanceCounts;
}

export interface FolderReport {
  /** Folder path relative to the specs root; "" for the root. */
  path: string;
  name: string;
  folders: FolderReport[];
  files: FileReport[];
  counts: ConformanceCounts;
}

export interface ConformanceReport {
  root: FolderReport;
  /** Files that hold no requirements at all (personas, flows, indexes). */
  filesWithoutRequirements: SpecFile[];
}

const STEP_PATTERN =
  /^\s*(?:[-*+]|\d+[.)])?\s*(?:\*\*|__)?(GIVEN|WHEN|THEN|AND|BUT)(?![A-Za-z])/u;
const NORMATIVE_PATTERN = /\b(?:SHALL|MUST)\b/u;

/** Checks the GIVEN/WHEN/THEN structure of one scenario's text. */
export function checkScenario(text: string): ScenarioIssue[] {
  let firstWhen = -1;
  let firstThen = -1;
  text.split("\n").forEach((line, index) => {
    const keyword = STEP_PATTERN.exec(line)?.[1];
    if (keyword === "WHEN" && firstWhen === -1) firstWhen = index;
    if (keyword === "THEN" && firstThen === -1) firstThen = index;
  });
  const issues: ScenarioIssue[] = [];
  if (firstWhen === -1) issues.push("missing-when");
  if (firstThen === -1) issues.push("missing-then");
  if (firstWhen !== -1 && firstThen !== -1 && firstThen < firstWhen) {
    issues.push("then-before-when");
  }
  return issues;
}

export function emptyCounts(): ConformanceCounts {
  return {
    requirements: 0,
    conformant: 0,
    withScenarios: 0,
    scenarios: 0,
    wellFormedScenarios: 0,
    missingId: 0,
    scenariosMissingId: 0,
    testedScenarios: 0,
    untestedScenarios: 0,
  };
}

function addCounts(into: ConformanceCounts, from: ConformanceCounts): void {
  into.requirements += from.requirements;
  into.conformant += from.conformant;
  into.withScenarios += from.withScenarios;
  into.scenarios += from.scenarios;
  into.wellFormedScenarios += from.wellFormedScenarios;
  into.missingId += from.missingId;
  into.scenariosMissingId += from.scenariosMissingId;
  into.testedScenarios += from.testedScenarios;
  into.untestedScenarios += from.untestedScenarios;
}

/** Share of conformant requirements, 0..100; null when there are none. */
export function conformancePercent(counts: ConformanceCounts): number | null {
  if (counts.requirements === 0) return null;
  return Math.round((counts.conformant / counts.requirements) * 100);
}

/** Structural issues of a scenario, without the test check. */
function scenarioIssues(node: SpecNode, text: string): ScenarioIssue[] {
  return [
    ...checkScenario(text),
    ...(node.specId ? [] : (["missing-id"] as const)),
  ];
}

const isStructural = (issue: ScenarioIssue): boolean =>
  issue !== "missing-id" && issue !== "untested";

/**
 * Checks one spec file. With `traceability`, scenarios must also be cited by
 * a test marker; without it (the report could not run), that check is off.
 */
export function checkFile(
  file: SpecFile,
  traceability?: Traceability,
): FileReport {
  const textByOwner = new Map<string, string>();
  for (const section of file.sections) {
    const own = section.nodeId === section.ownerId ? section.heading : "";
    textByOwner.set(
      section.ownerId,
      `${textByOwner.get(section.ownerId) ?? ""}${own}\n${section.body}`,
    );
  }

  const counts = emptyCounts();
  const requirements: RequirementReport[] = [];
  for (const node of file.nodes) {
    if (node.kind !== "spec") continue;
    const traced = node.specId ? traceability?.get(node.specId) : undefined;
    const needsTests = traceability !== undefined && isTestVerified(traced);
    const scenarios = file.nodes
      .filter(
        (child) => child.kind === "scenario" && child.parentId === node.id,
      )
      .map((scenario): ScenarioReport => {
        const cited =
          (scenario.specId
            ? traceability?.get(scenario.specId)?.citations
            : undefined) ?? [];
        const issues = scenarioIssues(
          scenario,
          textByOwner.get(scenario.id) ?? "",
        );
        // A scenario without an ID cannot be cited; it is already flagged.
        if (needsTests && scenario.specId && cited.length === 0) {
          issues.push("untested");
        }
        return { node: scenario, issues, citations: cited };
      });
    const wellFormed = scenarios.filter((s) =>
      s.issues.every((issue) => !isStructural(issue)),
    ).length;
    const scenariosMissingId = scenarios.filter((s) => !s.node.specId).length;
    const tested = scenarios.filter((s) => s.citations.length > 0).length;
    const untested = scenarios.filter((s) =>
      s.issues.includes("untested"),
    ).length;

    const issues: RequirementIssue[] = [];
    if (scenarios.length === 0) issues.push("no-scenarios");
    if (wellFormed < scenarios.length) issues.push("malformed-scenarios");
    if (!NORMATIVE_PATTERN.test(textByOwner.get(node.id) ?? "")) {
      issues.push("no-normative");
    }
    if (!node.specId) issues.push("missing-id");
    if (scenariosMissingId > 0) issues.push("scenarios-missing-id");
    if (untested > 0) issues.push("untested-scenarios");

    const method = traced?.method;
    requirements.push({
      node,
      scenarios,
      issues,
      ...(method && method !== "Test" ? { nonTestMethod: method } : {}),
    });
    counts.requirements += 1;
    if (issues.length === 0) counts.conformant += 1;
    if (scenarios.length > 0) counts.withScenarios += 1;
    counts.scenarios += scenarios.length;
    counts.wellFormedScenarios += wellFormed;
    if (!node.specId) counts.missingId += 1;
    counts.scenariosMissingId += scenariosMissingId;
    counts.testedScenarios += tested;
    counts.untestedScenarios += untested;
  }
  return { file, requirements, counts };
}

function folderOf(root: FolderReport, filePath: string): FolderReport {
  let folder = root;
  const segments = filePath.split("/").slice(0, -1);
  segments.forEach((segment, index) => {
    let child = folder.folders.find((candidate) => candidate.name === segment);
    if (!child) {
      child = {
        path: segments.slice(0, index + 1).join("/"),
        name: segment,
        folders: [],
        files: [],
        counts: emptyCounts(),
      };
      folder.folders.push(child);
    }
    folder = child;
  });
  return folder;
}

function sumCounts(folder: FolderReport): ConformanceCounts {
  const counts = emptyCounts();
  for (const child of folder.folders) addCounts(counts, sumCounts(child));
  for (const file of folder.files) addCounts(counts, file.counts);
  folder.counts = counts;
  return counts;
}

export function buildConformanceReport(
  catalog: SpecCatalog,
  traceability?: Traceability,
): ConformanceReport {
  const root: FolderReport = {
    path: "",
    name: "",
    folders: [],
    files: [],
    counts: emptyCounts(),
  };
  const filesWithoutRequirements: SpecFile[] = [];
  for (const file of catalog.files) {
    const report = checkFile(file, traceability);
    if (report.counts.requirements === 0) {
      filesWithoutRequirements.push(file);
    } else {
      folderOf(root, file.path).files.push(report);
    }
  }
  sumCounts(root);
  return { root, filesWithoutRequirements };
}
