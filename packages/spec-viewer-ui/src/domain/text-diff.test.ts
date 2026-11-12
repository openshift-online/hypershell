import { describe, expect, it } from "vitest";

import {
  diffBlocks,
  diffLines,
  diffWords,
  splitBlocks,
  splitLines,
  textsDiffer,
  type DiffSegment,
} from "./text-diff";

const changedText = (segments: DiffSegment[] | undefined): string[] =>
  (segments ?? []).filter((s) => s.changed).map((s) => s.text);

describe("diffWords", () => {
  it("marks only the words that differ", () => {
    const [before, after] = diffWords(
      "The gateway SHALL rotate certificates daily",
      "The gateway MUST rotate certificates hourly",
    );
    expect(changedText(before)).toEqual(["SHALL", "daily"]);
    expect(changedText(after)).toEqual(["MUST", "hourly"]);
    expect(before.map((s) => s.text).join("")).toBe(
      "The gateway SHALL rotate certificates daily",
    );
  });

  it("marks everything when a line is too long to compare", () => {
    const long = Array.from({ length: 1600 }, (_, i) => `w${String(i)}`).join(
      " ",
    );
    const [before] = diffWords(long, `${long} more`);
    expect(before).toEqual([{ text: long, changed: true }]);
  });
});

describe("textsDiffer", () => {
  it("ignores line endings and trailing space", () => {
    expect(textsDiffer("a  \r\nb\n\n", "a\nb")).toBe(false);
    expect(textsDiffer("a\nb", "a\nc")).toBe(true);
  });
});

describe("splitLines", () => {
  it("returns no lines for blank text", () => {
    expect(splitLines("\n\n")).toEqual([]);
  });

  it("collapses runs of blank lines and trims the ends", () => {
    expect(splitLines("\n a  \n\n \n\nb\n\n")).toEqual([" a", "", "b"]);
  });
});

describe("diffLines", () => {
  it("pairs a replaced line and keeps the unchanged ones on both sides", () => {
    const rows = diffLines("one\ntwo\nthree", "one\n2\nthree");
    expect(rows.map((row) => row.kind)).toEqual(["same", "changed", "same"]);
    expect(changedText(rows[1]?.before?.segments)).toEqual(["two"]);
    expect(changedText(rows[1]?.after?.segments)).toEqual(["2"]);
  });

  it("shows an inserted line on the right only", () => {
    const rows = diffLines("a\nc", "a\nb\nc");
    expect(rows.map((row) => row.kind)).toEqual(["same", "added", "same"]);
    expect(rows[1]?.before).toBeNull();
  });

  it("shows a deleted line on the left only", () => {
    const rows = diffLines("a\nb\nc", "a\nc");
    expect(rows.map((row) => row.kind)).toEqual(["same", "removed", "same"]);
    expect(rows[1]?.after).toBeNull();
    expect(changedText(rows[1]?.before?.segments)).toEqual(["b"]);
  });

  it("leaves the extra lines of an uneven change unpaired", () => {
    const rows = diffLines("a\nb\nc", "x");
    expect(rows.map((row) => row.kind)).toEqual([
      "changed",
      "removed",
      "removed",
    ]);
  });

  it("returns only unchanged rows for equal texts", () => {
    const rows = diffLines("a\nb", "a\nb\n");
    expect(rows.every((row) => row.kind === "same")).toBe(true);
    expect(rows).toHaveLength(2);
  });

  it("handles an empty side", () => {
    expect(diffLines("", "a").map((row) => row.kind)).toEqual(["added"]);
    expect(diffLines("a", "").map((row) => row.kind)).toEqual(["removed"]);
  });
});

describe("splitBlocks", () => {
  it("splits at blank lines but keeps a code fence whole", () => {
    expect(
      splitBlocks("# T\n\npara one\nstill\n\n```\na\n\nb\n```\n\n- x\n- y"),
    ).toEqual(["# T", "para one\nstill", "```\na\n\nb\n```", "- x\n- y"]);
  });
});

describe("diffBlocks", () => {
  it("pairs a reworded paragraph and keeps the others as they are", () => {
    const rows = diffBlocks(
      "# T\n\nOld text.\n\nEnd.",
      "# T\n\nNew text.\n\nEnd.",
    );
    expect(rows.map((row) => row.kind)).toEqual(["same", "changed", "same"]);
    expect(rows[1]?.before).toBe("Old text.");
    expect(rows[1]?.after).toBe("New text.");
  });

  it("shows an added block on the right only", () => {
    const rows = diffBlocks("a", "a\n\nb");
    expect(rows.map((row) => [row.kind, row.before])).toEqual([
      ["same", "a"],
      ["added", null],
    ]);
  });
});
