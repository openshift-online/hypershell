#!/usr/bin/env node
// Convert the web console BFF's NODE_V8_COVERAGE output from a Kind e2e run
// into istanbul data, and compare it with the BFF unit-test coverage.
// Spec: specs/platform/local-development.spec.md DEV-LOC-18.
//
// Usage: node scripts/code-coverage/bff_report.mjs <repo-root> <coverage-dir>
//
// Reads (under <coverage-dir>):
//   e2e/raw/node/*.json                  V8 coverage written by the BFF on exit
//   e2e/bff-dist/                        /app/dist copied from the coverage image
//                                        (compiled with inline source maps)
//   unit/frontend/bff/coverage-final.json  vitest output for the BFF unit tests
// Writes:
//   e2e/bff-coverage-final.json          istanbul data for the e2e run
//   e2e/bff-summary.json                 line totals: unit, e2e, both
//   report/bff-e2e/index.html            HTML report of the e2e run
//
// It uses the same parser (vitest/node parseAstAsync) and converter
// (ast-v8-to-istanbul) as `vitest --coverage`, resolved from the web console
// package, so unit and e2e numbers are measured the same way.

import fs from "node:fs";
import path from "node:path";
import { createRequire } from "node:module";
import { pathToFileURL } from "node:url";

const [repoRoot, coverageDir] = process.argv.slice(2);
if (!repoRoot || !coverageDir) {
  console.error("usage: bff_report.mjs <repo-root> <coverage-dir>");
  process.exit(2);
}

const consoleRequire = createRequire(path.join(repoRoot, "components/web-console/package.json"));
const coverageV8Require = createRequire(consoleRequire.resolve("@vitest/coverage-v8/package.json"));
const importFrom = (req, id) => import(pathToFileURL(req.resolve(id)).href);
const convert = (await importFrom(coverageV8Require, "ast-v8-to-istanbul")).default;
const libCoverage = coverageV8Require("istanbul-lib-coverage");
const libReport = coverageV8Require("istanbul-lib-report");
const reports = coverageV8Require("istanbul-reports");
const { parseAstAsync } = await importFrom(consoleRequire, "vitest/node");

// The image runs the BFF from /app/dist/bff/src; source maps point to /app/src.
const RUNTIME_PREFIX = "file:///app/dist/bff/src/";
const distSrc = path.join(coverageDir, "e2e/bff-dist/bff/src");
const rawDir = path.join(coverageDir, "e2e/raw/node");

// Merge the per-script function data from every coverage file (one per process).
const functionsByUrl = new Map();
for (const file of fs.existsSync(rawDir) ? fs.readdirSync(rawDir) : []) {
  if (!file.endsWith(".json")) continue;
  const { result } = JSON.parse(fs.readFileSync(path.join(rawDir, file), "utf8"));
  for (const script of result) {
    if (!script.url.startsWith(RUNTIME_PREFIX) || script.url.endsWith(".test.js")) continue;
    const list = functionsByUrl.get(script.url) ?? [];
    list.push(...script.functions);
    functionsByUrl.set(script.url, list);
  }
}

function listJs(dir) {
  return fs.readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) return listJs(full);
    return full.endsWith(".js") && !full.endsWith(".test.js") ? [full] : [];
  });
}

// Every compiled file is reported. Files the BFF never loaded count as 0%,
// which matches how the unit run counts untested files.
const e2eMap = libCoverage.createCoverageMap({});
let loaded = 0;
for (const jsFile of listJs(distSrc)) {
  const relative = path.relative(distSrc, jsFile).split(path.sep).join("/");
  const url = RUNTIME_PREFIX + relative;
  const code = fs.readFileSync(jsFile, "utf8");
  const sourceMap = JSON.parse(fs.readFileSync(`${jsFile}.map`, "utf8"));
  let functions = functionsByUrl.get(url);
  if (functions) {
    loaded += 1;
  } else {
    functions = [{ functionName: "", isBlockCoverage: true, ranges: [{ startOffset: 0, endOffset: code.length, count: 0 }] }];
  }
  e2eMap.merge(await convert({ code, ast: parseAstAsync(code), sourceMap, coverage: { url, functions } }));
}

// Compare by "src/<file>.ts" so the image paths (/app/src/...) and the local
// unit-test paths (.../components/web-console/bff/src/...) line up.
function normalise(key) {
  if (key.startsWith("/app/src/")) return key.slice("/app/".length);
  const index = key.lastIndexOf("/bff/src/");
  return index >= 0 ? `src/${key.slice(index + "/bff/src/".length)}` : null;
}
function coveredLines(map) {
  const out = {};
  for (const key of map.files()) {
    const name = normalise(key);
    if (!name) continue;
    const lines = {};
    for (const [line, hits] of Object.entries(map.fileCoverageFor(key).getLineCoverage())) lines[line] = hits > 0;
    out[name] = lines;
  }
  return out;
}

const unitPath = path.join(coverageDir, "unit/frontend/bff/coverage-final.json");
const unit = fs.existsSync(unitPath)
  ? coveredLines(libCoverage.createCoverageMap(JSON.parse(fs.readFileSync(unitPath, "utf8"))))
  : {};
const e2e = coveredLines(e2eMap);

const totals = { lines: 0, unit: 0, e2e: 0, both: 0 };
for (const file of new Set([...Object.keys(unit), ...Object.keys(e2e)])) {
  const lines = new Set([...Object.keys(unit[file] ?? {}), ...Object.keys(e2e[file] ?? {})]);
  for (const line of lines) {
    const inUnit = unit[file]?.[line] === true;
    const inE2e = e2e[file]?.[line] === true;
    totals.lines += 1;
    totals.unit += inUnit ? 1 : 0;
    totals.e2e += inE2e ? 1 : 0;
    totals.both += inUnit || inE2e ? 1 : 0;
  }
}

fs.writeFileSync(path.join(coverageDir, "e2e/bff-coverage-final.json"), JSON.stringify(e2eMap.toJSON()));
fs.writeFileSync(path.join(coverageDir, "e2e/bff-summary.json"), JSON.stringify(totals, null, 2));
const context = libReport.createContext({ dir: path.join(coverageDir, "report/bff-e2e"), coverageMap: e2eMap });
reports.create("html").execute(context);

const pct = (part) => (totals.lines ? ((100 * part) / totals.lines).toFixed(1) : "0.0");
console.log(`BFF scripts loaded during e2e: ${loaded} of ${listJs(distSrc).length}`);
console.log(`BFF lines: unit ${pct(totals.unit)}%, e2e ${pct(totals.e2e)}%, unit + e2e ${pct(totals.both)}%`);
