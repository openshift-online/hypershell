// Test traceability (STD-IDS-07/08): which tests carry a `Verifies: <ID>`
// marker for each requirement and scenario, and which requirements are
// proven without tests (`**Verification:** Inspection`).
//
// The server only returns raw marker lines. This module applies the same
// grammar as `scripts/check_spec_ids.py --report`, so the counts match
// `make traceability` without running Python.

import type { SpecFileSource } from "./spec-document";

export interface TestCitation {
  /** Repository-relative path of the test file. */
  path: string;
  /** 1-based line of the marker. */
  line: number;
  /** Test name after the marker, when one was found. */
  test: string;
}

export interface TracedItem {
  citations: TestCitation[];
  /**
   * Verification method of a requirement: "Test", or "Inspection",
   * "Analysis", or "Demonstration" for requirements proven without tests.
   */
  method?: string;
}

/** Traced requirements and scenarios by ID. */
export type Traceability = Map<string, TracedItem>;

/**
 * A line that starts a `Verifies:` comment, as the server found it. The
 * server adds the lines after it, up to and including the first line that
 * is neither blank nor a comment, so the test name can be read.
 */
export interface MarkerLine {
  path: string;
  /** 1-based line number. */
  line: number;
  text: string;
  following: string[];
}

const CODE = "[A-Z][A-Z0-9]{2,3}";
const REQ = `${CODE}-${CODE}-\\d{2,3}`;
const ID = `${REQ}(?:\\.\\d{2})?`;

const MARKER_PATTERN = new RegExp(
  `^\\s*(?://|#) Verifies: (${ID}(?:, ${ID})*)\\s*$`,
  "u",
);
const HEADING_START_PATTERN =
  /^(#{1,6})\s+(Requirement|Legacy compatibility|Scenario)\s*:/u;
const HEADING_PATTERN =
  /^(#{3,4}) (Requirement|Legacy compatibility|Scenario): (\S+) -- \S/u;
const REQUIREMENT_ID_PATTERN = new RegExp(`^${REQ}$`, "u");
const VERIFICATION_PATTERN = /^\*\*Verification:\*\*\s*([A-Za-z]+)/u;
const NON_TEST_METHODS = new Set(["inspection", "analysis", "demonstration"]);

const QUOTED = "[\"'`]([^\"'`]+)[\"'`]";
const NAME_PATTERNS = [
  /^\s*func\s+(?:\([^)]*\)\s*)?(\w+)\s*\(/u,
  new RegExp(`^\\s*(?:\\w+\\.)*(?:t|s|suite)\\.Run\\(\\s*${QUOTED}`, "u"),
  new RegExp(
    `^\\s*(?:await\\s+)?(?:test|it|describe|context|specify)(?:\\.\\w+)*(?:\\([^)]*\\))?\\s*\\(\\s*${QUOTED}`,
    "u",
  ),
  /^\s*(?:async\s+)?def\s+(\w+)\s*\(/u,
  /^\s*class\s+(\w+)/u,
  /^\s*(?:function\s+)?(\w+)\s*\(\)\s*\{?/u,
  /^\s*function\s+(\w+)/u,
];

const COMMENT_STARTS = ["//", "#", "/*", "*", "*/"];

/** True for a blank line or a comment line; a marker skips these. */
export function isCommentOrBlank(line: string): boolean {
  const stripped = line.trim();
  return (
    stripped === "" ||
    COMMENT_STARTS.some((start) => stripped.startsWith(start))
  );
}

/** IDs a well-formed marker cites; null for a malformed marker. */
export function parseMarker(text: string): string[] | null {
  const match = MARKER_PATTERN.exec(text);
  return match?.[1] ? match[1].split(", ") : null;
}

/** Name of the test that follows a marker. */
export function testName(following: readonly string[]): string {
  const code = following.find((line) => !isCommentOrBlank(line));
  if (code === undefined) return "";
  for (const pattern of NAME_PATTERNS) {
    const name = pattern.exec(code)?.[1];
    if (name) return name;
  }
  return code.trim().slice(0, 100);
}

/**
 * Verification methods other than "Test", by requirement ID. A
 * `**Verification:**` line counts for the requirement above it until the
 * next heading of level 3 or higher that is not a requirement or scenario.
 */
export function verificationMethods(content: string): Map<string, string> {
  const methods = new Map<string, string>();
  let current: string | undefined;
  let inFence = false;
  for (const line of content.split("\n")) {
    const stripped = line.trimStart();
    if (stripped.startsWith("```") || stripped.startsWith("~~~")) {
      inFence = !inFence;
      continue;
    }
    if (inFence) continue;
    const isSpecHeading = HEADING_START_PATTERN.test(line);
    if (line.startsWith("#")) {
      const level = line.length - line.replace(/^#+/u, "").length;
      if (level <= 3 && !isSpecHeading) current = undefined;
    }
    if (isSpecHeading) {
      // As in the checker: scenario and malformed headings keep the
      // requirement; a `###` requirement heading with a bad ID ends it.
      const [, hashes, keyword, id] = HEADING_PATTERN.exec(line) ?? [];
      if (hashes === "###" && keyword !== "Scenario" && id !== undefined) {
        current = REQUIREMENT_ID_PATTERN.test(id) ? id : undefined;
      }
      continue;
    }
    const method = VERIFICATION_PATTERN.exec(line)?.[1];
    if (current && method && NON_TEST_METHODS.has(method.toLowerCase())) {
      methods.set(
        current,
        method.charAt(0).toUpperCase() + method.slice(1).toLowerCase(),
      );
    }
  }
  return methods;
}

/**
 * Builds traceability from the spec files and the marker lines. Malformed
 * markers cite nothing; `make check` reports them.
 */
export function buildTraceability(
  specFiles: readonly SpecFileSource[],
  markers: readonly MarkerLine[],
): Traceability {
  const traced: Traceability = new Map();
  const entry = (id: string): TracedItem => {
    let item = traced.get(id);
    if (!item) {
      item = { citations: [] };
      traced.set(id, item);
    }
    return item;
  };
  for (const file of specFiles) {
    if (!file.path.endsWith(".spec.md")) continue;
    for (const [id, method] of verificationMethods(file.content)) {
      entry(id).method = method;
    }
  }
  for (const marker of markers) {
    const ids = parseMarker(marker.text);
    if (!ids) continue;
    const test = testName(marker.following);
    for (const id of ids) {
      entry(id).citations.push({ path: marker.path, line: marker.line, test });
    }
  }
  return traced;
}

/** True when a requirement is proven by tests (the default method). */
export function isTestVerified(item: TracedItem | undefined): boolean {
  return (item?.method ?? "Test") === "Test";
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

/** Validates the `api/markers` response. */
export function parseMarkerLines(body: unknown): MarkerLine[] {
  if (!Array.isArray(body)) {
    throw new Error("Unexpected marker response: not a list");
  }
  return body.map((item): MarkerLine => {
    if (
      !isRecord(item) ||
      typeof item.path !== "string" ||
      typeof item.line !== "number" ||
      typeof item.text !== "string" ||
      !Array.isArray(item.following) ||
      !item.following.every((line) => typeof line === "string")
    ) {
      throw new Error("Unexpected marker response: malformed entry");
    }
    return {
      path: item.path,
      line: item.line,
      text: item.text,
      following: item.following,
    };
  });
}
