/*
 * A small synchronous Cedar tokenizer for read-only policy source. It mirrors
 * the Monarch rules the web console registers for the PatternFly code editor
 * (cedar-monaco.ts), so Cedar is colored the same way in the editor, the
 * policy detail pages, and the gateway Policies tab. Tokens render as React
 * elements with CSS classes; no raw HTML is produced.
 */

export const cedarKeywords: readonly string[] = [
  "forbid",
  "permit",
  "when",
  "unless",
  "if",
  "then",
  "else",
  "in",
  "has",
  "like",
  "is",
];

export const cedarVariables: readonly string[] = [
  "principal",
  "action",
  "resource",
  "context",
];

export const cedarLiterals: readonly string[] = ["true", "false"];

export type CedarTokenKind =
  | "annotation"
  | "comment"
  | "keyword"
  | "number"
  | "operator"
  | "plain"
  | "string"
  | "type"
  | "variable";

export interface CedarToken {
  kind: CedarTokenKind;
  value: string;
}

const rules: readonly (readonly [RegExp, CedarTokenKind | "word"])[] = [
  [/^\/\/[^\n]*/u, "comment"],
  [/^@[A-Za-z_]\w*/u, "annotation"],
  [/^"(?:[^"\\\n]|\\.)*"?/u, "string"],
  [/^\d+/u, "number"],
  [/^[A-Z]\w*(?=::)/u, "type"],
  [/^[A-Za-z_]\w*/u, "word"],
  [/^(?:&&|\|\||==|!=|<=|>=|[<>!])/u, "operator"],
];

const keywordSet = new Set([...cedarKeywords, ...cedarLiterals]);
const variableSet = new Set(cedarVariables);

function wordKind(word: string): CedarTokenKind {
  if (keywordSet.has(word)) {
    return "keyword";
  }
  return variableSet.has(word) ? "variable" : "plain";
}

/** Splits Cedar source into colored tokens; the values join to the input. */
export function tokenizeCedar(source: string): CedarToken[] {
  const tokens: CedarToken[] = [];
  const push = (kind: CedarTokenKind, value: string) => {
    const last = tokens.at(-1);
    if (last?.kind === kind && kind === "plain") {
      last.value += value;
    } else {
      tokens.push({ kind, value });
    }
  };
  let rest = source;
  while (rest.length > 0) {
    let matched = false;
    for (const [pattern, kind] of rules) {
      const value = pattern.exec(rest)?.[0];
      if (value) {
        push(kind === "word" ? wordKind(value) : kind, value);
        rest = rest.slice(value.length);
        matched = true;
        break;
      }
    }
    if (!matched) {
      push("plain", rest.charAt(0));
      rest = rest.slice(1);
    }
  }
  return tokens;
}
