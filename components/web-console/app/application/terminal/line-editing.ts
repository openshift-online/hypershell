/*
 * Pure line-editing operations for the terminal prompt (WEB-TERM-05). Each
 * takes the input value and cursor position and returns the edited value and
 * the new cursor position.
 */

export interface LineEditState {
  readonly cursor: number;
  readonly value: string;
}

function clampCursor(value: string, cursor: number): number {
  return Math.min(value.length, Math.max(0, cursor));
}

/** Ctrl+U: delete from the cursor to the start of the line. */
export function deleteToLineStart({ cursor, value }: LineEditState) {
  const at = clampCursor(value, cursor);
  return { cursor: 0, value: value.slice(at) };
}

/** Ctrl+K: delete from the cursor to the end of the line. */
export function deleteToLineEnd({ cursor, value }: LineEditState) {
  const at = clampCursor(value, cursor);
  return { cursor: at, value: value.slice(0, at) };
}

/**
 * Ctrl+W, Alt+Backspace, Ctrl+Backspace: delete the word before the cursor,
 * with any whitespace between it and the cursor.
 */
export function deleteWordBefore({ cursor, value }: LineEditState) {
  const at = clampCursor(value, cursor);
  let start = at;
  while (start > 0 && /\s/u.test(value.charAt(start - 1))) {
    start -= 1;
  }
  while (start > 0 && !/\s/u.test(value.charAt(start - 1))) {
    start -= 1;
  }
  return { cursor: start, value: value.slice(0, start) + value.slice(at) };
}

export interface CommandRecallState {
  /** Draft typed before recall started, restored after the newest entry. */
  readonly draft: string;
  /** Index into the recall list, or null when editing the draft. */
  readonly index: number | null;
}

export const initialCommandRecall: CommandRecallState = Object.freeze({
  draft: "",
  index: null,
});

/**
 * ArrowUp/ArrowDown: moves through earlier commands (oldest first in
 * `commands`). Returns the new recall state and the value to show, or null
 * when there is nothing to recall in that direction.
 */
export function recallCommand(
  commands: readonly string[],
  state: CommandRecallState,
  currentValue: string,
  direction: "newer" | "older",
): { state: CommandRecallState; value: string } | null {
  if (commands.length === 0) {
    return null;
  }
  if (direction === "older") {
    if (state.index === 0) {
      return null;
    }
    const index = state.index === null ? commands.length - 1 : state.index - 1;
    const draft = state.index === null ? currentValue : state.draft;
    return { state: { draft, index }, value: commands[index] ?? "" };
  }
  if (state.index === null) {
    return null;
  }
  if (state.index >= commands.length - 1) {
    return { state: initialCommandRecall, value: state.draft };
  }
  const index = state.index + 1;
  return { state: { draft: state.draft, index }, value: commands[index] ?? "" };
}
