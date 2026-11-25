/*
 * The terminal's shared, in-memory history (WEB-TERM-07, WEB-UDOC-05).
 *
 * One chronological list of lines holds typed commands, runbook cell runs, and
 * their output. It is a plain immutable value: the presentation layer keeps it
 * in memory only and never writes it to browser storage, a cache, or a URL,
 * because command output can contain one-time credentials (WEB-DATA-02).
 * Localized notices are stored as typed values and formatted at render time.
 */

/** The log keeps at most this many lines (WEB-TERM-07). */
export const terminalHistoryLineLimit = 5000;

export type TerminalOutputStream = "stderr" | "stdout";

export type TerminalEntrySource = "runbook" | "typed";

export type TerminalNotice =
  | { readonly kind: "cancelled" }
  | { readonly kind: "exit-code"; readonly exitCode: number }
  | { readonly kind: "incomplete-line" }
  | { readonly kind: "interrupted" }
  | { readonly kind: "not-hsctl"; readonly program: string }
  | { readonly kind: "runtime-error" }
  | { readonly kind: "session-expired" }
  | { readonly kind: "shell-operators" };

interface LineBase {
  /** Position within the batch of lines appended together (for stagger). */
  readonly batchPosition: number;
  readonly entryId: string;
  /** Stable React key; never reused within a page session. */
  readonly id: string;
  /** Monotonic line number within the page session. */
  readonly sequence: number;
}

export type TerminalLine =
  | (LineBase & {
      readonly kind: "command";
      readonly source: TerminalEntrySource;
      readonly text: string;
    })
  | (LineBase & {
      readonly kind: "output";
      readonly stream: TerminalOutputStream;
      readonly text: string;
    })
  | (LineBase & {
      readonly kind: "notice";
      readonly notice: TerminalNotice;
    });

type NewLine =
  | { readonly kind: "command"; source: TerminalEntrySource; text: string }
  | { readonly kind: "notice"; notice: TerminalNotice }
  | { readonly kind: "output"; stream: TerminalOutputStream; text: string };

export interface TerminalHistory {
  /** Lines removed from the top to respect the line limit. */
  readonly droppedLineCount: number;
  readonly lines: readonly TerminalLine[];
  readonly nextEntryNumber: number;
  readonly nextSequence: number;
}

export const emptyTerminalHistory: TerminalHistory = Object.freeze({
  droppedLineCount: 0,
  lines: Object.freeze([]),
  nextEntryNumber: 1,
  nextSequence: 1,
});

/**
 * Splits runtime output into display lines. A final newline does not create an
 * empty trailing line; everything else is kept exactly as returned.
 */
export function splitOutputLines(text: string): string[] {
  if (text.length === 0) {
    return [];
  }
  const lines = text.split("\n");
  if (lines.at(-1) === "") {
    lines.pop();
  }
  return lines;
}

function appendLines(
  history: TerminalHistory,
  entryId: string,
  newLines: readonly NewLine[],
  limit: number,
): TerminalHistory {
  if (newLines.length === 0) {
    return history;
  }
  let sequence = history.nextSequence;
  const appended = newLines.map((line, batchPosition): TerminalLine => {
    const current = sequence;
    sequence += 1;
    return Object.freeze({
      ...line,
      batchPosition,
      entryId,
      id: `line-${String(current)}`,
      sequence: current,
    });
  });
  const combined = [...history.lines, ...appended];
  const excess = Math.max(0, combined.length - limit);
  return Object.freeze({
    droppedLineCount: history.droppedLineCount + excess,
    lines: Object.freeze(excess > 0 ? combined.slice(excess) : combined),
    nextEntryNumber: history.nextEntryNumber,
    nextSequence: sequence,
  });
}

/** Starts a new entry with its command line and returns the entry id. */
export function startTerminalEntry(
  history: TerminalHistory,
  source: TerminalEntrySource,
  command: string,
  limit = terminalHistoryLineLimit,
): { entryId: string; history: TerminalHistory } {
  const entryId = `entry-${String(history.nextEntryNumber)}`;
  const withEntry: TerminalHistory = {
    ...history,
    nextEntryNumber: history.nextEntryNumber + 1,
  };
  return {
    entryId,
    history: appendLines(
      withEntry,
      entryId,
      [{ kind: "command", source, text: command }],
      limit,
    ),
  };
}

/** Appends output lines (already split) to an entry. */
export function appendTerminalOutput(
  history: TerminalHistory,
  entryId: string,
  lines: readonly string[],
  stream: TerminalOutputStream,
  limit = terminalHistoryLineLimit,
): TerminalHistory {
  return appendLines(
    history,
    entryId,
    lines.flatMap((line) =>
      splitOutputLines(line.length === 0 ? "\n" : line).map(
        (text) => ({ kind: "output", stream, text }) as const,
      ),
    ),
    limit,
  );
}

/** Appends a localized notice to an entry. */
export function appendTerminalNotice(
  history: TerminalHistory,
  entryId: string,
  notice: TerminalNotice,
  limit = terminalHistoryLineLimit,
): TerminalHistory {
  return appendLines(history, entryId, [{ kind: "notice", notice }], limit);
}

/**
 * Clears every line while keeping the counters, so line keys and entry ids
 * stay unique for the rest of the page session.
 */
export function clearTerminalHistory(
  history: TerminalHistory,
): TerminalHistory {
  return Object.freeze({
    droppedLineCount: 0,
    lines: Object.freeze([]),
    nextEntryNumber: history.nextEntryNumber,
    nextSequence: history.nextSequence,
  });
}
