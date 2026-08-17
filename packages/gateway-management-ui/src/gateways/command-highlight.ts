import { createHighlighter, type Highlighter } from "shiki";

// High-contrast GitHub themes: each pairs its own foreground and background for
// AA contrast, so the highlighted block stays readable in both PatternFly
// themes and clears the axe contrast checks.
const lightTheme = "github-light-high-contrast";
const darkTheme = "github-dark-high-contrast";

// Every command block is shell script, so a single-language, dual-theme
// highlighter is enough. Create it once and share it across blocks.
let highlighterPromise: Promise<Highlighter> | undefined;

function getHighlighter(): Promise<Highlighter> {
  highlighterPromise ??= createHighlighter({
    langs: ["bash"],
    themes: [lightTheme, darkTheme],
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
