import { execFileSync } from "node:child_process";
import path from "node:path";

import { buildTraceability, isTestVerified } from "../src/domain/traceability";
import { readSpecFiles } from "./read-specs";
import { createRepository, findMarkers, WORKING_TREE } from "./repository";

interface ReportCitation {
  location: string;
  test: string;
}
interface ReportScenario {
  id: string;
  citations: ReportCitation[];
}
interface ReportRequirement extends ReportScenario {
  method: string;
  scenarios: ReportScenario[];
}
interface Report {
  files: { requirements: ReportRequirement[] }[];
}

const ROOT = path.resolve(import.meta.dirname, "../../..");

const fromReport = (citations: ReportCitation[]): string[] =>
  citations.map((cite) => `${cite.location} ${cite.test}`).sort();

// The viewer reimplements the marker grammar of scripts/check_spec_ids.py so
// it can run where Python cannot (the fleet dashboard image). This test runs
// both over the real repository and fails on any difference.
describe("traceability parity with make traceability", () => {
  it("cites the same tests and methods as the checker", async () => {
    const markers = await findMarkers(
      await createRepository(ROOT).tree(WORKING_TREE),
    );
    const specs = await readSpecFiles(path.join(ROOT, "specs"));
    const traced = buildTraceability(specs, markers);
    const report = JSON.parse(
      execFileSync(
        "python3",
        ["scripts/check_spec_ids.py", "--report", "--format", "json"],
        {
          cwd: ROOT,
          env: { ...process.env, PYTHONDONTWRITEBYTECODE: "1" },
          maxBuffer: 64 * 1024 * 1024,
        },
      ).toString(),
    ) as Report;

    const differences: string[] = [];
    const compare = (id: string, expected: ReportCitation[]) => {
      const actual = (traced.get(id)?.citations ?? [])
        .map((cite) => `${cite.path}:${String(cite.line)} ${cite.test}`)
        .sort();
      if (actual.join("|") !== fromReport(expected).join("|")) {
        differences.push(id);
      }
    };
    for (const file of report.files) {
      for (const requirement of file.requirements) {
        if (
          (requirement.method === "Test") !==
          isTestVerified(traced.get(requirement.id))
        ) {
          differences.push(`${requirement.id} method`);
        }
        compare(requirement.id, requirement.citations);
        for (const scenario of requirement.scenarios) {
          compare(scenario.id, scenario.citations);
        }
      }
    }
    expect(differences).toEqual([]);
  }, 120_000);
});
