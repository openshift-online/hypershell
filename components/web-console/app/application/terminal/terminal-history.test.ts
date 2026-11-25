import { describe, expect, it } from "vitest";

import {
  appendTerminalNotice,
  appendTerminalOutput,
  clearTerminalHistory,
  emptyTerminalHistory,
  splitOutputLines,
  startTerminalEntry,
  terminalHistoryLineLimit,
} from "./terminal-history";

describe("terminal history", () => {
  // Verifies: WEB-TERM-07
  it("keeps one chronological list of typed and runbook entries", () => {
    const typed = startTerminalEntry(
      emptyTerminalHistory,
      "typed",
      "list gateways",
    );
    const runbook = startTerminalEntry(
      typed.history,
      "runbook",
      "hsctl create gateway",
    );
    const history = appendTerminalOutput(
      runbook.history,
      typed.entryId,
      ["late output"],
      "stdout",
    );
    expect(typed.entryId).toBe("entry-1");
    expect(runbook.entryId).toBe("entry-2");
    expect(history.lines.map((line) => [line.kind, line.entryId])).toEqual([
      ["command", "entry-1"],
      ["command", "entry-2"],
      ["output", "entry-1"],
    ]);
    expect(history.lines[1]).toMatchObject({ source: "runbook" });
  });

  // Verifies: WEB-TERM-07
  it("drops the oldest lines beyond the limit and counts them", () => {
    let history = startTerminalEntry(
      emptyTerminalHistory,
      "typed",
      "a",
    ).history;
    history = appendTerminalOutput(
      history,
      "entry-1",
      Array.from({ length: 6 }, (_value, index) => `line ${String(index)}`),
      "stdout",
      5,
    );
    expect(history.lines).toHaveLength(5);
    expect(history.droppedLineCount).toBe(2);
    expect(history.lines[0]).toMatchObject({ text: "line 1" });
    expect(terminalHistoryLineLimit).toBe(5000);
  });

  // Verifies: WEB-TERM-08
  it("gives lines stable unique keys and their position in the batch", () => {
    const started = startTerminalEntry(emptyTerminalHistory, "typed", "x");
    const history = appendTerminalOutput(
      started.history,
      started.entryId,
      ["a\nb", "c"],
      "stderr",
    );
    expect(history.lines.map((line) => line.id)).toEqual([
      "line-1",
      "line-2",
      "line-3",
      "line-4",
    ]);
    expect(history.lines.map((line) => line.batchPosition)).toEqual([
      0, 0, 1, 2,
    ]);
    const cleared = clearTerminalHistory(history);
    expect(cleared.lines).toEqual([]);
    const next = startTerminalEntry(cleared, "typed", "y");
    expect(next.entryId).toBe("entry-2");
    expect(next.history.lines[0]?.id).toBe("line-5");
  });

  // Verifies: WEB-TERM-04
  it("splits output without inventing a trailing empty line", () => {
    expect(splitOutputLines("")).toEqual([]);
    expect(splitOutputLines("a\n")).toEqual(["a"]);
    expect(splitOutputLines("a\n\nb")).toEqual(["a", "", "b"]);
    const started = startTerminalEntry(emptyTerminalHistory, "typed", "x");
    expect(
      appendTerminalOutput(started.history, started.entryId, [""], "stdout")
        .lines[1],
    ).toMatchObject({ text: "" });
    expect(
      appendTerminalOutput(started.history, started.entryId, [], "stdout"),
    ).toBe(started.history);
    expect(
      appendTerminalNotice(started.history, started.entryId, {
        exitCode: 2,
        kind: "exit-code",
      }).lines[1],
    ).toMatchObject({ kind: "notice", notice: { exitCode: 2 } });
  });
});
