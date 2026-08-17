import { createHighlighter, type Highlighter } from "shiki";
import { createJavaScriptRegexEngine } from "shiki/engine/javascript";

// High-contrast GitHub themes: each pairs its own foreground and background for
// AA contrast, so the highlighted block stays readable in both PatternFly
// themes and clears the axe contrast checks.
const lightTheme = "github-light-high-contrast";
const darkTheme = "github-dark-high-contrast";

// Every command block is shell script, so a single-language, dual-theme
// highlighter is enough. Create it once and share it across blocks.
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

/**
 * Render a shell command as dual-theme Shiki HTML.
 *
 * The output carries light colors inline plus `--shiki-dark` CSS variables that
 * the dark theme activates; the raw `command` string still drives copy, so the
 * highlighted markup is display-only.
 */
export async function highlightCommand(command: string): Promise<string> {
  const highlighter = await getHighlighter();
  return highlighter.codeToHtml(command, {
    lang: "bash",
    themes: { light: lightTheme, dark: darkTheme },
  });
}
