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
// attributes, so that markup would render uncolored. Rewrite each token's inline
// colors to `hl-fg-<hex>` / `hl-dfg-<hex>` classes and drop the container's
// inline style; the self-hosted stylesheet (gateway-connection-steps.module.css)
// paints those classes, so highlighting works under the CSP with no relaxation.
// Keep that stylesheet's palette in sync with the themes -- command-highlight
// tests fail if Shiki emits a color without a matching rule.
const HEX = "([0-9a-fA-F]{3,8})";
const lightColor = new RegExp(`(?:^|;)\\s*color:\\s*#${HEX}`);
const darkColor = new RegExp(`--shiki-dark:\\s*#${HEX}`);

function tokenClasses(style: string): string {
  const classes: string[] = [];
  const light = lightColor.exec(style)?.[1];
  const dark = darkColor.exec(style)?.[1];
  if (light) {
    classes.push(`hl-fg-${light.toLowerCase()}`);
  }
  if (dark) {
    classes.push(`hl-dfg-${dark.toLowerCase()}`);
  }
  if (/font-weight:\s*bold/.test(style)) {
    classes.push("hl-bold");
  }
  if (/font-style:\s*italic/.test(style)) {
    classes.push("hl-italic");
  }
  return classes.join(" ");
}

function toClassBasedMarkup(html: string): string {
  return html
    .replace(/(<pre\b[^>]*?)\s+style="[^"]*"/g, "$1")
    .replace(/<span style="([^"]*)">/g, (_match, style: string) => {
      const classes = tokenClasses(style);
      return classes ? `<span class="${classes}">` : "<span>";
    });
}

/**
 * Render a shell command as dual-theme, CSP-safe Shiki HTML.
 *
 * Token colors are carried by `hl-fg-*` / `hl-dfg-*` classes (no inline styles),
 * so the markup survives the BFF's `style-src-attr 'none'` policy. The raw
 * `command` string still drives copy, so the highlighted markup is display-only.
 */
export async function highlightCommand(command: string): Promise<string> {
  const highlighter = await getHighlighter();
  const html = highlighter.codeToHtml(command, {
    lang: "bash",
    themes: { light: lightTheme, dark: darkTheme },
  });
  return toClassBasedMarkup(html);
}
