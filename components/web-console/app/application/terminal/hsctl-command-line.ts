/*
 * Command-line parsing for the browser hsctl terminal (WEB-TERM-04).
 *
 * The terminal runs no shell. A line is split into words the way a POSIX shell
 * splits words (single quotes, double quotes, and backslash escapes), and any
 * construct that would make a shell do more than pass words to one program is
 * refused instead of being passed through as text.
 */

/**
 * Top-level `hsctl` commands, as registered by `components/cli/cmd/hsctl`
 * (generated: completion, config, create, delete, get, list, login, logout,
 * update, version, whoami; hand-written: apply, revoke, secret-source, tui;
 * and cobra's built-in help).
 */
export const hsctlTopLevelCommands = [
  "apply",
  "completion",
  "config",
  "create",
  "delete",
  "get",
  "help",
  "list",
  "login",
  "logout",
  "revoke",
  "secret-source",
  "tui",
  "update",
  "version",
  "whoami",
] as const;

export type HsctlTopLevelCommand = (typeof hsctlTopLevelCommands)[number];

const topLevelCommandSet: ReadonlySet<string> = new Set(hsctlTopLevelCommands);

export function isHsctlTopLevelCommand(
  word: string,
): word is HsctlTopLevelCommand {
  return topLevelCommandSet.has(word);
}

export type CommandLineWords =
  | { readonly kind: "words"; readonly words: readonly string[] }
  /** An unquoted pipe, redirection, `;`, `&`, `$(`, or backtick. */
  | { readonly kind: "shell-operator" }
  /** An unterminated quote or a trailing backslash. */
  | { readonly kind: "incomplete" };

const unquotedOperators: ReadonlySet<string> = new Set([
  "|",
  "<",
  ">",
  ";",
  "&",
  "`",
]);

// Inside double quotes a backslash escapes only these characters (POSIX).
const doubleQuoteEscapable: ReadonlySet<string> = new Set([
  "$",
  "`",
  '"',
  "\\",
  "\n",
]);

const whitespace: ReadonlySet<string> = new Set([" ", "\t", "\n", "\r"]);

/**
 * Splits one input line into words. Command substitution (`$(` and backtick)
 * is refused inside double quotes too, because a POSIX shell expands it there;
 * passing it through as text would silently differ from what the user typed in
 * a shell.
 */
export function splitCommandLine(line: string): CommandLineWords {
  const words: string[] = [];
  let current: string | null = null;
  let quote: "double" | "none" | "single" = "none";
  const characters = Array.from(line);

  for (let index = 0; index < characters.length; index += 1) {
    const character = characters[index] ?? "";
    const next = characters[index + 1];

    if (quote === "single") {
      if (character === "'") {
        quote = "none";
      } else {
        current = `${current ?? ""}${character}`;
      }
      continue;
    }

    if (quote === "double") {
      if (character === '"') {
        quote = "none";
      } else if (character === "`" || (character === "$" && next === "(")) {
        return { kind: "shell-operator" };
      } else if (
        character === "\\" &&
        next !== undefined &&
        doubleQuoteEscapable.has(next)
      ) {
        current = `${current ?? ""}${next === "\n" ? "" : next}`;
        index += 1;
      } else {
        current = `${current ?? ""}${character}`;
      }
      continue;
    }

    if (whitespace.has(character)) {
      if (current !== null) {
        words.push(current);
        current = null;
      }
    } else if (character === "'") {
      quote = "single";
      current ??= "";
    } else if (character === '"') {
      quote = "double";
      current ??= "";
    } else if (character === "\\") {
      if (next === undefined) {
        return { kind: "incomplete" };
      }
      current = `${current ?? ""}${next === "\n" ? "" : next}`;
      index += 1;
    } else if (
      unquotedOperators.has(character) ||
      (character === "$" && next === "(")
    ) {
      return { kind: "shell-operator" };
    } else {
      current = `${current ?? ""}${character}`;
    }
  }

  if (quote !== "none") {
    return { kind: "incomplete" };
  }
  if (current !== null) {
    words.push(current);
  }
  return { kind: "words", words };
}

export type CommandLineAnalysis =
  | { readonly kind: "empty" }
  | { readonly kind: "clear" }
  | { readonly kind: "shell-operator" }
  | { readonly kind: "incomplete" }
  | { readonly kind: "not-hsctl"; readonly program: string }
  /** `argv` excludes the program name, as the runtime bridge expects. */
  | { readonly kind: "run"; readonly argv: readonly string[] };

/**
 * Decides what one input line asks for: nothing, clearing the log, a refusal,
 * or an `hsctl` invocation. A leading `hsctl` word is optional.
 */
export function analyzeCommandLine(line: string): CommandLineAnalysis {
  const split = splitCommandLine(line);
  if (split.kind !== "words") {
    return split;
  }
  const [first, ...rest] = split.words;
  if (first === undefined) {
    return { kind: "empty" };
  }
  if (first === "clear" && rest.length === 0) {
    return { kind: "clear" };
  }
  if (first === "hsctl") {
    return { argv: rest, kind: "run" };
  }
  if (isHsctlTopLevelCommand(first)) {
    return { argv: split.words, kind: "run" };
  }
  return { kind: "not-hsctl", program: first };
}
