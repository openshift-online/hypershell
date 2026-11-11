import type { Element, ElementContent, Root } from "hast";

import type { DiffSegment } from "../domain/text-diff";

export const WORD_CLASS = "sv-diff__word";

function changedRanges(segments: DiffSegment[]): [number, number][] {
  const ranges: [number, number][] = [];
  let offset = 0;
  for (const segment of segments) {
    if (segment.changed) ranges.push([offset, offset + segment.text.length]);
    offset += segment.text.length;
  }
  return ranges;
}

function mark(text: string): Element {
  return {
    type: "element",
    tagName: "mark",
    properties: { className: [WORD_CLASS] },
    children: [{ type: "text", value: text }],
  };
}

function splitRange(
  text: string,
  ranges: [number, number][],
  start: number,
): ElementContent[] {
  const end = start + text.length;
  const parts: ElementContent[] = [];
  let cursor = start;
  for (const [from, to] of ranges) {
    const a = Math.max(from, cursor);
    const b = Math.min(to, end);
    if (a >= b) continue;
    if (a > cursor) {
      parts.push({
        type: "text",
        value: text.slice(cursor - start, a - start),
      });
    }
    parts.push(mark(text.slice(a - start, b - start)));
    cursor = b;
  }
  if (parts.length === 0) return [{ type: "text", value: text }];
  if (cursor < end) {
    parts.push({ type: "text", value: text.slice(cursor - start) });
  }
  return parts;
}

function markTree(root: Root, ranges: [number, number][]): void {
  let offset = 0;
  const visit = (parent: Root | Element): void => {
    const children: ElementContent[] = [];
    for (const child of parent.children) {
      if (child.type === "text") {
        children.push(...splitRange(child.value, ranges, offset));
        offset += child.value.length;
        continue;
      }
      if (child.type === "element") visit(child);
      if (child.type === "element") children.push(child);
    }
    parent.children = children;
  };
  visit(root);
}

/** Marks the changed ranges described by `segments` in a rendered tree. */
export function markSegments(root: Root, segments: DiffSegment[]): Root {
  markTree(root, changedRanges(segments));
  return root;
}
