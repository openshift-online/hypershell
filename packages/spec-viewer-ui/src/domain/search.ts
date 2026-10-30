// Spec search. A query is a list of clauses, and every clause must match:
//
//   word            fuzzy: a substring, an in-order character subsequence of
//                   a heading (abbreviation), or a word a letter or two off
//   "some phrase"   the literal text, ignoring case and line breaks
//   [two words]     all the words, in any order, on one source line
//
// Clauses are matched against a scope. A scenario's scope is its own text.
// A spec's scope is its own text plus its scenarios, so the clauses of one
// query may be found in different places of the same spec. Text that no spec
// or scenario encloses is matched, and reported, under its nearest heading.

import type { SpecCatalog } from "./spec-document";
import { plainHeading } from "./spec-document";

export interface Snippet {
  text: string;
  /** Half-open [start, end) ranges of matched text within `text`. */
  highlights: [number, number][];
}

export interface SearchHit {
  nodeId: string;
  score: number;
  snippet?: Snippet;
  /** Lowercase text this hit matched, including typo-corrected words. */
  terms: string[];
}

export type QueryClause =
  | { kind: "word"; term: string }
  | { kind: "phrase"; text: string }
  | { kind: "line"; words: string[] };

interface IndexedLine {
  text: string;
  /** Offset of the line in the part body; -1 for the heading line. */
  offset: number;
}

/** One section of text: a heading and the body up to the next heading. */
interface IndexedPart {
  nodeId: string;
  body: string;
  headingLower: string;
  bodyLower: string;
  /** Body with every run of whitespace collapsed to one space. */
  flatBodyLower: string;
  lines: IndexedLine[];
  words: string[];
}

interface Owner {
  nodeId: string;
  parts: IndexedPart[];
  /** Scenario owners of a spec; empty for other owners. */
  scenarioIds: string[];
}

export interface SearchIndex {
  owners: Map<string, Owner>;
}

const MIN_WORD_LENGTH = 4;
const SNIPPET_BEFORE = 60;
const SNIPPET_LENGTH = 200;
/** Weight of a match outside the scope's own heading section. */
const NESTED_WEIGHT = 0.75;

function indexLines(headingLower: string, body: string): IndexedLine[] {
  const lines: IndexedLine[] = [{ text: headingLower, offset: -1 }];
  let offset = 0;
  for (const line of body.split("\n")) {
    if (line.trim() !== "") {
      lines.push({ text: line.toLowerCase(), offset });
    }
    offset += line.length + 1;
  }
  return lines;
}

export function buildSearchIndex(catalog: SpecCatalog): SearchIndex {
  const owners = new Map<string, Owner>();
  for (const file of catalog.files) {
    for (const section of file.sections) {
      const headingLower = plainHeading(section.heading).toLowerCase();
      const bodyLower = section.body.toLowerCase();
      const words = new Set(
        `${headingLower} ${bodyLower}`
          .split(/[^\p{L}\p{N}_]+/u)
          .filter((word) => word.length >= MIN_WORD_LENGTH - 1),
      );
      const part: IndexedPart = {
        nodeId: section.nodeId,
        body: section.body,
        headingLower,
        bodyLower,
        flatBodyLower: bodyLower.replace(/\s+/gu, " "),
        lines: indexLines(headingLower, section.body),
        words: [...words],
      };
      const owner = owners.get(section.ownerId) ?? {
        nodeId: section.ownerId,
        parts: [],
        scenarioIds: [],
      };
      owner.parts.push(part);
      owners.set(section.ownerId, owner);
    }
  }
  for (const owner of owners.values()) {
    const node = catalog.nodesById.get(owner.nodeId);
    if (node?.kind !== "scenario" || !node.parentId) continue;
    const parent = owners.get(node.parentId);
    if (catalog.nodesById.get(node.parentId)?.kind === "spec" && parent) {
      parent.scenarioIds.push(owner.nodeId);
    }
  }
  return { owners };
}

/** Splits a query into word, "phrase", and [same line] clauses. */
export function parseQuery(query: string): QueryClause[] {
  const lower = query.toLowerCase();
  const clauses: QueryClause[] = [];
  let index = 0;
  const readUntil = (close: string): string => {
    const end = lower.indexOf(close, index + 1);
    const stop = end === -1 ? lower.length : end;
    const text = lower.slice(index + 1, stop);
    index = stop + 1;
    return text;
  };
  while (index < lower.length) {
    const char = lower[index] ?? "";
    if (/\s/u.test(char)) {
      index += 1;
    } else if (char === '"') {
      const text = readUntil('"').replace(/\s+/gu, " ").trim();
      if (text !== "") clauses.push({ kind: "phrase", text });
    } else if (char === "[") {
      const words = readUntil("]")
        .replace(/"/gu, " ")
        .split(/\s+/u)
        .filter((word) => word !== "");
      if (words.length > 0) clauses.push({ kind: "line", words });
    } else {
      let end = index;
      while (end < lower.length && !/[\s"[]/u.test(lower[end] ?? "")) {
        end += 1;
      }
      const term = lower.slice(index, end).replace(/\]/gu, "");
      if (term !== "") clauses.push({ kind: "word", term });
      index = end;
    }
  }
  return clauses;
}

/** Levenshtein distance, abandoned early once it exceeds `max`. */
export function editDistance(a: string, b: string, max: number): number {
  if (Math.abs(a.length - b.length) > max) return max + 1;
  let previous = Array.from({ length: b.length + 1 }, (_, i) => i);
  for (let i = 1; i <= a.length; i += 1) {
    const row = [i];
    let rowMin = i;
    for (let j = 1; j <= b.length; j += 1) {
      const cost = a[i - 1] === b[j - 1] ? 0 : 1;
      const value = Math.min(
        (previous[j] ?? 0) + 1,
        (row[j - 1] ?? 0) + 1,
        (previous[j - 1] ?? 0) + cost,
      );
      row.push(value);
      rowMin = Math.min(rowMin, value);
    }
    if (rowMin > max) return max + 1;
    previous = row;
  }
  return previous[b.length] ?? max + 1;
}

/**
 * Scores `term` as an in-order subsequence of `text`. Tighter matches score
 * higher; matches spread over more than three times the term length fail.
 */
export function subsequenceScore(term: string, text: string): number {
  let best = 0;
  let start = text.indexOf(term[0] ?? "");
  while (start !== -1) {
    let position = start;
    let matched = true;
    for (let i = 1; i < term.length; i += 1) {
      position = text.indexOf(term[i] ?? "", position + 1);
      if (position === -1) {
        matched = false;
        break;
      }
    }
    if (!matched) break;
    const span = position - start + 1;
    if (span <= term.length * 3) {
      best = Math.max(best, term.length / span);
    }
    start = text.indexOf(term[0] ?? "", start + 1);
  }
  return best;
}

function allowedTypos(term: string): number {
  if (term.length >= 8) return 2;
  if (term.length >= MIN_WORD_LENGTH) return 1;
  return 0;
}

function isWordStart(text: string, index: number): boolean {
  return index === 0 || !/[\p{L}\p{N}]/u.test(text[index - 1] ?? "");
}

/**
 * Builds a short excerpt of `body` around `anchor`, or around the first
 * highlighted term when no anchor is given.
 */
export function buildSnippet(
  body: string,
  terms: string[],
  anchor?: number,
  contextBefore = SNIPPET_BEFORE,
): Snippet | undefined {
  const lower = body.toLowerCase();
  let first = anchor ?? -1;
  if (anchor === undefined) {
    for (const term of terms) {
      const index = lower.indexOf(term);
      if (index !== -1 && (first === -1 || index < first)) first = index;
    }
  }
  if (first === -1) return undefined;

  let start = Math.max(0, first - contextBefore);
  if (start > 0) {
    const space = body.indexOf(" ", start);
    if (space !== -1 && space < first) start = space + 1;
  }
  const end = Math.min(body.length, start + SNIPPET_LENGTH);
  const raw = body.slice(start, end);
  const text =
    (start > 0 ? "..." : "") +
    raw
      .replace(/\*\*|__|`/gu, "")
      .replace(/\s+/gu, " ")
      .trim() +
    (end < body.length ? "..." : "");

  const textLower = text.toLowerCase();
  const ranges: [number, number][] = [];
  for (const term of terms) {
    if (term === "") continue;
    let index = textLower.indexOf(term);
    while (index !== -1) {
      ranges.push([index, index + term.length]);
      index = textLower.indexOf(term, index + term.length);
    }
  }
  ranges.sort((a, b) => a[0] - b[0]);
  const highlights: [number, number][] = [];
  for (const range of ranges) {
    const last = highlights.at(-1);
    if (last && range[0] <= last[1]) {
      last[1] = Math.max(last[1], range[1]);
    } else {
      highlights.push([range[0], range[1]]);
    }
  }
  return { text, highlights };
}

interface ClauseMatch {
  score: number;
  /** Lowercase strings to highlight in the snippet. */
  highlights: string[];
  /** Offset in the part body that a snippet should show. */
  anchor?: number;
  /** True when `anchor` starts the matching line, so the snippet starts there. */
  isLine?: boolean;
}

function matchWord(term: string, part: IndexedPart): ClauseMatch | null {
  const inHeading = part.headingLower.indexOf(term);
  const inBody = part.bodyLower.indexOf(term);
  if (inHeading !== -1) {
    return {
      score: isWordStart(part.headingLower, inHeading) ? 12 : 9,
      highlights: [term],
      ...(inBody !== -1 ? { anchor: inBody } : {}),
    };
  }
  if (inBody !== -1) {
    return {
      score: isWordStart(part.bodyLower, inBody) ? 4 : 3,
      highlights: [term],
      anchor: inBody,
    };
  }
  // Keep the better of an abbreviation match on the heading and a typo match
  // on any word; a typo of a heading word beats a loose subsequence.
  let best: ClauseMatch | null = null;
  if (term.length >= 2) {
    const fuzzy = subsequenceScore(term, part.headingLower);
    if (fuzzy > 0) best = { score: 4 * fuzzy, highlights: [] };
  }
  const typos = allowedTypos(term);
  if (typos > 0) {
    for (const word of part.words) {
      if (editDistance(term, word, typos) > typos) continue;
      const score = part.headingLower.includes(word) ? 6 : 2;
      if (!best || score > best.score) {
        const at = part.bodyLower.indexOf(word);
        best = {
          score,
          highlights: [word],
          ...(at !== -1 ? { anchor: at } : {}),
        };
      }
      if (score === 6) break;
    }
  }
  return best;
}

const escapeRegExp = (text: string): string =>
  text.replace(/[.*+?^${}()|[\]\\]/gu, "\\$&");

function matchPhrase(text: string, part: IndexedPart): ClauseMatch | null {
  const inHeading = part.headingLower.includes(text);
  if (!inHeading && !part.flatBodyLower.includes(text)) return null;
  // The phrase may wrap across source lines; find it in the raw body.
  const pattern = new RegExp(escapeRegExp(text).replace(/ /gu, "\\s+"), "u");
  const at = pattern.exec(part.bodyLower)?.index;
  return {
    score: inHeading ? 14 : 6,
    highlights: [text],
    ...(at !== undefined ? { anchor: at } : {}),
  };
}

function matchLine(words: string[], part: IndexedPart): ClauseMatch | null {
  for (const line of part.lines) {
    if (words.every((word) => line.text.includes(word))) {
      return {
        score: line.offset === -1 ? 14 : 8,
        highlights: words,
        ...(line.offset === -1 ? {} : { anchor: line.offset, isLine: true }),
      };
    }
  }
  return null;
}

function matchClause(clause: QueryClause, part: IndexedPart) {
  switch (clause.kind) {
    case "word":
      return matchWord(clause.term, part);
    case "phrase":
      return matchPhrase(clause.text, part);
    case "line":
      return matchLine(clause.words, part);
  }
}

interface ScopeMatch {
  score: number;
  snippet?: Snippet;
  terms: string[];
}

/** Matches every clause somewhere in `parts`; null when any clause fails. */
function matchScope(
  rootId: string,
  parts: IndexedPart[],
  clauses: QueryClause[],
): ScopeMatch | null {
  let score = 0;
  const highlights: string[] = [];
  let snippetSource: {
    part: IndexedPart;
    anchor: number;
    isLine: boolean;
    score: number;
  } | null = null;

  for (const clause of clauses) {
    let best: { match: ClauseMatch; part: IndexedPart; score: number } | null =
      null;
    for (const part of parts) {
      const match = matchClause(clause, part);
      if (!match) continue;
      const weighted =
        part.nodeId === rootId ? match.score : match.score * NESTED_WEIGHT;
      if (!best || weighted > best.score) {
        best = { match, part, score: weighted };
      }
    }
    if (!best) return null;
    score += best.score;
    highlights.push(...best.match.highlights);
    const { anchor } = best.match;
    if (
      anchor !== undefined &&
      (!snippetSource || best.score > snippetSource.score)
    ) {
      snippetSource = {
        part: best.part,
        anchor,
        isLine: best.match.isLine === true,
        score: best.score,
      };
    }
  }

  const words = clauses.flatMap((clause) =>
    clause.kind === "word" ? [clause.term] : [],
  );
  if (words.length > 1) {
    const phrase = words.join(" ");
    const root = parts.find((part) => part.nodeId === rootId);
    if (root?.headingLower.includes(phrase)) score += 10;
    else if (parts.some((part) => part.flatBodyLower.includes(phrase))) {
      score += 5;
    }
  }

  const snippet = snippetSource
    ? buildSnippet(
        snippetSource.part.body,
        highlights,
        snippetSource.anchor,
        snippetSource.isLine ? 0 : SNIPPET_BEFORE,
      )
    : undefined;
  return {
    score,
    terms: [...new Set(highlights)],
    ...(snippet ? { snippet } : {}),
  };
}

/** Literal text of every clause: words, phrases, and same-line words. */
export function queryTerms(query: string): string[] {
  const terms = parseQuery(query).flatMap((clause) => {
    switch (clause.kind) {
      case "word":
        return [clause.term];
      case "phrase":
        return [clause.text];
      case "line":
        return clause.words;
    }
  });
  return [...new Set(terms)];
}

/**
 * Text to highlight in documents for a search: the query's own terms plus
 * the words the hits actually matched, so typo matches are marked too.
 */
export function highlightTerms(query: string, hits: SearchHit[]): string[] {
  const terms = new Set(queryTerms(query));
  for (const hit of hits) {
    for (const term of hit.terms) terms.add(term);
  }
  return [...terms];
}

export function searchSpecs(
  index: SearchIndex,
  query: string,
  limit = 300,
): SearchHit[] {
  const clauses = parseQuery(query);
  if (clauses.length === 0) return [];

  const hits = new Map<string, SearchHit>();
  for (const owner of index.owners.values()) {
    const match = matchScope(owner.nodeId, owner.parts, clauses);
    if (match) hits.set(owner.nodeId, { nodeId: owner.nodeId, ...match });
  }

  // A spec also matches when its clauses are spread over its own text and
  // its scenarios. When one scenario already matches on its own, that
  // scenario is the more precise result and the spec is only its parent.
  for (const owner of index.owners.values()) {
    if (owner.scenarioIds.length === 0 || hits.has(owner.nodeId)) continue;
    if (owner.scenarioIds.some((id) => hits.has(id))) continue;
    const parts = [
      ...owner.parts,
      ...owner.scenarioIds.flatMap((id) => index.owners.get(id)?.parts ?? []),
    ];
    const match = matchScope(owner.nodeId, parts, clauses);
    if (match) hits.set(owner.nodeId, { nodeId: owner.nodeId, ...match });
  }

  return [...hits.values()].sort((a, b) => b.score - a.score).slice(0, limit);
}
