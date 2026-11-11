// Marks the words that differ between two rendered blocks. The markdown is
// rendered first and its visible text compared afterward, so a change inside
// bold text, a link, or a code span cannot break the markup.

import { diffWords, type DiffSegment } from "../domain/text-diff";

export const WORD_CLASS = "sv-diff__word";

function textNodesOf(root: Node): Text[] {
  const found: Text[] = [];
  const walker = root.ownerDocument?.createTreeWalker(
    root,
    NodeFilter.SHOW_TEXT,
  );
  for (let node = walker?.nextNode(); node; node = walker?.nextNode()) {
    found.push(node as Text);
  }
  return found;
}

/** Wraps the changed runs of `segments` in the text nodes, in place. */
function mark(nodes: Text[], segments: DiffSegment[]): void {
  // Changed character ranges over the concatenated text.
  const ranges: [number, number][] = [];
  let offset = 0;
  for (const segment of segments) {
    if (segment.changed) ranges.push([offset, offset + segment.text.length]);
    offset += segment.text.length;
  }
  let start = 0;
  for (const node of nodes) {
    const text = node.data;
    const end = start + text.length;
    const pieces: (string | HTMLElement)[] = [];
    let cursor = 0;
    for (const [from, to] of ranges) {
      const a = Math.max(from, start) - start;
      const b = Math.min(to, end) - start;
      if (a >= b) continue;
      if (a > cursor) pieces.push(text.slice(cursor, a));
      const word = node.ownerDocument.createElement("mark");
      word.className = WORD_CLASS;
      word.textContent = text.slice(a, b);
      pieces.push(word);
      cursor = b;
    }
    if (pieces.length > 0) {
      if (cursor < text.length) pieces.push(text.slice(cursor));
      node.replaceWith(...pieces);
    }
    start = end;
  }
}

function parse(html: string): HTMLElement {
  return new DOMParser().parseFromString(`<body>${html}</body>`, "text/html")
    .body;
}

/** The two blocks' HTML with the changed words wrapped in `<mark>`. */
export function markChangedWords(
  beforeHtml: string,
  afterHtml: string,
): [before: string, after: string] {
  const before = parse(beforeHtml);
  const after = parse(afterHtml);
  const beforeNodes = textNodesOf(before);
  const afterNodes = textNodesOf(after);
  const [left, right] = diffWords(
    beforeNodes.map((node) => node.data).join(""),
    afterNodes.map((node) => node.data).join(""),
  );
  mark(beforeNodes, left);
  mark(afterNodes, right);
  return [before.innerHTML, after.innerHTML];
}
