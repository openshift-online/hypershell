import { createHighlighter, type Highlighter } from "shiki";
import { createJavaScriptRegexEngine } from "shiki/engine/javascript";

const lightTheme = "github-light-high-contrast";
const darkTheme = "github-dark-high-contrast";

// High-contrast GitHub themes: each pairs its own foreground and background for
// AA contrast, so the highlighted block stays readable in both PatternFly
// themes and clears the axe contrast checks.
//
// Use Shiki's JavaScript regex engine rather than the default Oniguruma WASM
// engine: the web-console BFF serves a hardened CSP without `wasm-unsafe-eval`,
// so `WebAssembly.instantiate` is blocked in the browser and the WASM engine
// would silently fail (leaving the plain, unhighlighted fallback). The JS engine
// needs no WASM and handles the bash grammar fully.
let highlighterPromise: Promise<Highlighter> | undefined;

function getHighlighter(): Promise<Highlighter> {
  highlighterPromise ??= createHighlighter({
    langs: ["bash"],
    themes: [lightTheme, darkTheme],
    engine: createJavaScriptRegexEngine(),
  });
  return highlighterPromise;
}

// Shiki colors every token with an inline `style` attribute (exposing the dark
// palette via a `--shiki-dark` custom property). The web-console BFF serves a
// hardened CSP with `style-src-attr 'none'`, which strips inline style
// attributes, so that markup would render uncolored. Instead of emitting HTML,
// we work from Shiki's structured tokens and carry each token's colors as
// `hl-fg-<hex>` / `hl-dfg-<hex>` classes; the self-hosted stylesheet
// (gateway-connection-steps.module.css) paints those classes, so highlighting
// works under the CSP with no relaxation. Keep that stylesheet's palette in sync
// with the themes -- command-highlight tests fail if Shiki emits a color without
// a matching rule.
function colorClass(
  prefix: string,
  value: string | undefined,
): string | undefined {
  const hex = /^#([0-9a-fA-F]{3,8})$/.exec(value?.trim() ?? "")?.[1];
  return hex ? `${prefix}-${hex.toLowerCase()}` : undefined;
}

function tokenClassName(style: Record<string, string> | undefined): string {
  if (!style) {
    return "";
  }
  const classes: string[] = [];
  const fg = colorClass("hl-fg", style.color);
  const dfg = colorClass("hl-dfg", style["--shiki-dark"]);
  if (fg) {
    classes.push(fg);
  }
  if (dfg) {
    classes.push(dfg);
  }
  if ((style["font-weight"] ?? "").includes("bold")) {
    classes.push("hl-bold");
  }
  if ((style["font-style"] ?? "").includes("italic")) {
    classes.push("hl-italic");
  }
  return classes.join(" ");
}

/**
 * A run of the highlighted command. `text` parts carry static, syntax-colored
 * text; `field` parts mark where an editable value was substituted (via a unique
 * marker) so the renderer can drop an inline editor in that slot while keeping
 * the surrounding token color.
 */
export type CommandPart =
  | { className: string; kind: "text"; value: string }
  | { className: string; kind: "field"; marker: string };

// Split a single token's text on any of the edit markers, emitting a `field`
// part for each marker occurrence (carrying the token's color so the editor
// matches the surrounding syntax) and `text` parts for the runs between them.
function splitOnMarkers(
  content: string,
  markers: readonly string[],
  className: string,
): CommandPart[] {
  const parts: CommandPart[] = [];
  let rest = content;
  while (rest.length > 0) {
    let earliest = -1;
    let matched = "";
    for (const marker of markers) {
      const index = rest.indexOf(marker);
      if (index !== -1 && (earliest === -1 || index < earliest)) {
        earliest = index;
        matched = marker;
      }
    }
    if (earliest === -1) {
      parts.push({ className, kind: "text", value: rest });
      break;
    }
    if (earliest > 0) {
      parts.push({ className, kind: "text", value: rest.slice(0, earliest) });
    }
    parts.push({ className, kind: "field", marker: matched });
    rest = rest.slice(earliest + matched.length);
  }
  return parts;
}

/**
 * Highlight a shell command that embeds edit markers, returning an ordered list
 * of parts: syntax-colored static text and field slots at each marker.
 *
 * The whole command is highlighted at once (so bash tokenizes flags, arguments,
 * and operators in context), then each token's text is split at the markers.
 * Because markers are placed where a whole argument word would go, they tokenize
 * as their own argument-colored tokens, so the resulting field slots inherit the
 * correct color and no HTML tag is ever broken mid-element.
 */
export async function highlightTemplate(
  command: string,
  markers: readonly string[],
): Promise<CommandPart[]> {
  const highlighter = await getHighlighter();
  const { tokens } = highlighter.codeToTokens(command, {
    lang: "bash",
    themes: { light: lightTheme, dark: darkTheme },
  });

  const parts: CommandPart[] = [];
  tokens.forEach((line, index) => {
    if (index > 0) {
      parts.push({ className: "", kind: "text", value: "\n" });
    }
    for (const token of line) {
      const className = tokenClassName(token.htmlStyle);
      parts.push(...splitOnMarkers(token.content, markers, className));
    }
  });
  return parts;
}
