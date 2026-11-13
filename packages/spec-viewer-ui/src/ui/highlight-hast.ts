import type { Element, ElementContent, Root } from "hast";

import { transformTextNodes } from "./hast-utils";

export const MATCH_CLASS = "sv-match";

const escapeRegExp = (text: string): string =>
  text.replace(/[.*+?^${}()|[\]\\]/gu, "\\$&");

function mark(text: string): Element {
  return {
    type: "element",
    tagName: "mark",
    properties: { className: [MATCH_CLASS] },
    children: [{ type: "text", value: text }],
  };
}

function split(text: string, pattern: RegExp): ElementContent[] {
  const parts: ElementContent[] = [];
  let last = 0;
  for (const match of text.matchAll(pattern)) {
    parts.push({ type: "text", value: text.slice(last, match.index) });
    parts.push(mark(match[0]));
    last = match.index + match[0].length;
  }
  if (parts.length === 0) return [{ type: "text", value: text }];
  parts.push({ type: "text", value: text.slice(last) });
  return parts;
}

/**
 * Wraps every case-insensitive occurrence of `terms` in HAST text with a
 * PatternFly-styled mark. Terms shorter than two characters are ignored.
 */
export function highlightHast(root: Root, terms: string[]): Root {
  const usable = terms
    .map((term) => term.trim())
    .filter((term) => term.length >= 2)
    .sort((a, b) => b.length - a.length);
  if (usable.length === 0) return root;
  const pattern = new RegExp(
    usable.map((term) => escapeRegExp(term).replace(/ /gu, "\\s+")).join("|"),
    "giu",
  );
  transformTextNodes(root, (text) => split(text, pattern));
  return root;
}
