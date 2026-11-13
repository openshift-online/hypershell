// Text diff for the branch comparison. It lines two versions of a text up
// like a GitHub split view: whole lines first, then, inside a line that
// changed, the words that differ.

/** A run of text in one line; `changed` marks the words that differ. */
export interface DiffSegment {
  text: string;
  changed: boolean;
}

export interface DiffLine {
  segments: DiffSegment[];
}

/**
 * One row of the split view. `same` rows hold both lines; a `changed` row
 * pairs a removed line with the line that replaced it; `removed` and `added`
 * rows have only one side.
 */
export interface DiffRow {
  kind: "same" | "changed" | "removed" | "added";
  before: DiffLine | null;
  after: DiffLine | null;
}

type Op = "equal" | "delete" | "insert";

/** Past this many table cells, the texts are not lined up. */
const MAX_CELLS = 4_000_000;
/** Lines with more words than this are shown as wholly changed. */
const MAX_WORDS = 1500;

/**
 * Edit script from `before` to `after`, by longest common subsequence. The
 * shared start and end are cut off first, which is most of any real change.
 * Inputs too large for the table are reported as fully replaced.
 */
function editScript(before: string[], after: string[]): Op[] {
  let start = 0;
  while (
    start < before.length &&
    start < after.length &&
    before[start] === after[start]
  ) {
    start += 1;
  }
  let endBefore = before.length;
  let endAfter = after.length;
  while (
    endBefore > start &&
    endAfter > start &&
    before[endBefore - 1] === after[endAfter - 1]
  ) {
    endBefore -= 1;
    endAfter -= 1;
  }
  const a = before.slice(start, endBefore);
  const b = after.slice(start, endAfter);
  const ops: Op[] = Array.from({ length: start }, () => "equal");

  if ((a.length + 1) * (b.length + 1) > MAX_CELLS) {
    ops.push(...a.map((): Op => "delete"), ...b.map((): Op => "insert"));
  } else {
    // lcs[i][j]: length of the common subsequence of a[i..] and b[j..].
    const width = b.length + 1;
    const lcs = new Uint32Array((a.length + 1) * width);
    for (let i = a.length - 1; i >= 0; i -= 1) {
      for (let j = b.length - 1; j >= 0; j -= 1) {
        lcs[i * width + j] =
          a[i] === b[j]
            ? (lcs[(i + 1) * width + j + 1] ?? 0) + 1
            : Math.max(
                lcs[(i + 1) * width + j] ?? 0,
                lcs[i * width + j + 1] ?? 0,
              );
      }
    }
    let i = 0;
    let j = 0;
    while (i < a.length || j < b.length) {
      if (i < a.length && j < b.length && a[i] === b[j]) {
        ops.push("equal");
        i += 1;
        j += 1;
      } else if (
        j >= b.length ||
        (i < a.length &&
          (lcs[(i + 1) * width + j] ?? 0) >= (lcs[i * width + j + 1] ?? 0))
      ) {
        ops.push("delete");
        i += 1;
      } else {
        ops.push("insert");
        j += 1;
      }
    }
  }
  ops.push(
    ...Array.from({ length: before.length - endBefore }, (): Op => "equal"),
  );
  return ops;
}

const WORD = /\s+|[\p{L}\p{N}_]+|[^\s\p{L}\p{N}_]/gu;

function tokens(line: string): string[] {
  return line.match(WORD) ?? [];
}

/** Joins neighbouring runs that share a flag. */
function mergeSegments(parts: DiffSegment[]): DiffSegment[] {
  const merged: DiffSegment[] = [];
  for (const part of parts) {
    const last = merged.at(-1);
    if (last?.changed === part.changed) last.text += part.text;
    else merged.push({ ...part });
  }
  return merged;
}

/**
 * Marks the words of two paired lines that differ. A line that is too long
 * to compare by word is marked as changed throughout.
 */
export function diffWords(
  before: string,
  after: string,
): [before: DiffSegment[], after: DiffSegment[]] {
  const a = tokens(before);
  const b = tokens(after);
  if (a.length > MAX_WORDS || b.length > MAX_WORDS) {
    return [
      before === "" ? [] : [{ text: before, changed: true }],
      after === "" ? [] : [{ text: after, changed: true }],
    ];
  }
  const left: DiffSegment[] = [];
  const right: DiffSegment[] = [];
  let i = 0;
  let j = 0;
  for (const op of editScript(a, b)) {
    if (op === "equal") {
      left.push({ text: a[i] ?? "", changed: false });
      right.push({ text: b[j] ?? "", changed: false });
      i += 1;
      j += 1;
    } else if (op === "delete") {
      left.push({ text: a[i] ?? "", changed: true });
      i += 1;
    } else {
      right.push({ text: b[j] ?? "", changed: true });
      j += 1;
    }
  }
  return [mergeSegments(left), mergeSegments(right)];
}

/**
 * Lines of a text. Line endings, trailing spaces, and the number of blank
 * lines in a row do not count, and blank lines at either end are dropped.
 */
export function splitLines(text: string): string[] {
  const lines: string[] = [];
  for (const raw of text.replace(/\r\n?/gu, "\n").split("\n")) {
    const line = raw.trimEnd();
    if (line === "" && (lines.length === 0 || lines.at(-1) === "")) continue;
    lines.push(line);
  }
  if (lines.at(-1) === "") lines.pop();
  return lines;
}

/** True when the texts differ in anything but the spacing `splitLines` drops. */
export function textsDiffer(before: string, after: string): boolean {
  return splitLines(before).join("\n") !== splitLines(after).join("\n");
}

const plain = (text: string): DiffSegment[] =>
  text === "" ? [] : [{ text, changed: false }];
const whole = (text: string): DiffSegment[] =>
  text === "" ? [] : [{ text, changed: true }];

type RowKind = DiffRow["kind"];

interface AlignedRow {
  kind: RowKind;
  before: string | null;
  after: string | null;
}

/**
 * Lines two sequences up. Within a run of changes, the n-th removed item
 * pairs with the n-th added item, as GitHub does.
 */
function align(a: string[], b: string[]): AlignedRow[] {
  const rows: AlignedRow[] = [];
  let i = 0;
  let j = 0;
  const ops = editScript(a, b);
  let index = 0;
  while (index < ops.length) {
    if (ops[index] === "equal") {
      rows.push({ kind: "same", before: a[i] ?? "", after: b[j] ?? "" });
      i += 1;
      j += 1;
      index += 1;
      continue;
    }
    // A run of deletes and inserts up to the next unchanged item.
    const removed: string[] = [];
    const added: string[] = [];
    while (index < ops.length && ops[index] !== "equal") {
      if (ops[index] === "delete") removed.push(a[i++] ?? "");
      else added.push(b[j++] ?? "");
      index += 1;
    }
    const paired = Math.min(removed.length, added.length);
    for (let k = 0; k < paired; k += 1) {
      rows.push({
        kind: "changed",
        before: removed[k] ?? "",
        after: added[k] ?? "",
      });
    }
    for (let k = paired; k < removed.length; k += 1) {
      rows.push({ kind: "removed", before: removed[k] ?? "", after: null });
    }
    for (let k = paired; k < added.length; k += 1) {
      rows.push({ kind: "added", before: null, after: added[k] ?? "" });
    }
  }
  return rows;
}

/** Lines the two texts up for a split view, marking changed words. */
export function diffLines(before: string, after: string): DiffRow[] {
  return align(splitLines(before), splitLines(after)).map((row): DiffRow => {
    if (row.kind === "changed") {
      const [left, right] = diffWords(row.before ?? "", row.after ?? "");
      return {
        kind: "changed",
        before: { segments: left },
        after: { segments: right },
      };
    }
    const side = (text: string | null): DiffLine | null =>
      text === null
        ? null
        : { segments: row.kind === "same" ? plain(text) : whole(text) };
    return { kind: row.kind, before: side(row.before), after: side(row.after) };
  });
}

/** One row of the rendered split view: a markdown block on each side. */
export interface BlockRow {
  kind: RowKind;
  before: string | null;
  after: string | null;
}

/**
 * Markdown blocks of a text: runs of lines between blank lines. A fenced
 * code block stays whole even when it holds blank lines.
 */
export function splitBlocks(text: string): string[] {
  const blocks: string[] = [];
  let current: string[] = [];
  let fence: string | null = null;
  const flush = () => {
    if (current.length > 0) blocks.push(current.join("\n"));
    current = [];
  };
  for (const line of splitLines(text)) {
    const marker = /^\s*(`{3,}|~{3,})/u.exec(line)?.[1];
    if (marker) {
      if (fence === null) fence = marker.charAt(0);
      else if (marker.startsWith(fence)) fence = null;
    }
    if (line === "" && fence === null) flush();
    else current.push(line);
  }
  flush();
  return blocks;
}

/** Lines the markdown blocks of two texts up, whole block against block. */
export function diffBlocks(before: string, after: string): BlockRow[] {
  return align(splitBlocks(before), splitBlocks(after));
}
