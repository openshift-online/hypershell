import type { HighlighterCore } from "shiki/core";

import type { SyntaxHighlighterPort } from "../application/ports";
import type { SourceLanguage } from "../domain/references";

const LIGHT_THEME = "github-light-high-contrast";
const DARK_THEME = "github-dark-high-contrast";
/** Coloring very large files would stall the page; show them plain. */
const MAX_HIGHLIGHT_BYTES = 300_000;

// Same Shiki version, JavaScript regex engine, and high-contrast GitHub
// themes as the gateway management UI. Languages load on first use, and the
// whole highlighter is a separate chunk so search loads without it.
const LANGUAGES = {
  go: () => import("shiki/langs/go.mjs"),
  typescript: () => import("shiki/langs/typescript.mjs"),
  tsx: () => import("shiki/langs/tsx.mjs"),
  javascript: () => import("shiki/langs/javascript.mjs"),
  jsx: () => import("shiki/langs/jsx.mjs"),
  yaml: () => import("shiki/langs/yaml.mjs"),
  json: () => import("shiki/langs/json.mjs"),
  markdown: () => import("shiki/langs/markdown.mjs"),
  shellscript: () => import("shiki/langs/shellscript.mjs"),
  python: () => import("shiki/langs/python.mjs"),
  make: () => import("shiki/langs/make.mjs"),
  dockerfile: () => import("shiki/langs/dockerfile.mjs"),
  proto: () => import("shiki/langs/proto.mjs"),
  sql: () => import("shiki/langs/sql.mjs"),
  toml: () => import("shiki/langs/toml.mjs"),
  css: () => import("shiki/langs/css.mjs"),
  html: () => import("shiki/langs/html.mjs"),
} satisfies Record<Exclude<SourceLanguage, "text">, () => Promise<unknown>>;

let highlighter: Promise<HighlighterCore> | undefined;

function getHighlighter(): Promise<HighlighterCore> {
  highlighter ??= (async () => {
    const [{ createHighlighterCore }, { createJavaScriptRegexEngine }] =
      await Promise.all([
        import("shiki/core"),
        import("shiki/engine/javascript"),
      ]);
    return createHighlighterCore({
      themes: [
        import("shiki/themes/github-light-high-contrast.mjs"),
        import("shiki/themes/github-dark-high-contrast.mjs"),
      ],
      langs: [],
      engine: createJavaScriptRegexEngine(),
    });
  })();
  return highlighter;
}

const escapeHtml = (text: string): string =>
  text.replace(/&/gu, "&amp;").replace(/</gu, "&lt;").replace(/>/gu, "&gt;");

/** Uncolored rendering with the same line structure Shiki produces. */
function plainHtml(code: string): string {
  const lines = code
    .split("\n")
    .map((line) => `<span class="line">${escapeHtml(line)}</span>`)
    .join("\n");
  return `<pre class="shiki"><code>${lines}</code></pre>`;
}

export const shikiHighlighter: SyntaxHighlighterPort = {
  async highlight(code, language) {
    if (language === "text" || code.length > MAX_HIGHLIGHT_BYTES) {
      return plainHtml(code);
    }
    const instance = await getHighlighter();
    if (!instance.getLoadedLanguages().includes(language)) {
      await instance.loadLanguage(await LANGUAGES[language]());
    }
    return instance.codeToHtml(code, {
      lang: language,
      themes: { light: LIGHT_THEME, dark: DARK_THEME },
    });
  },
};
