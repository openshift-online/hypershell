import { useCallback, useEffect, useLayoutEffect, useState } from "react";

export type Theme = "light" | "dark";

const THEME_KEY = "hypershell-spec-viewer.theme";
const DARK_CLASS = "pf-v6-theme-dark";
const DARK_QUERY = "(prefers-color-scheme: dark)";

/** The theme chosen with the header button; null follows the system. */
function readChoice(): Theme | null {
  try {
    const stored = window.localStorage.getItem(THEME_KEY);
    return stored === "light" || stored === "dark" ? stored : null;
  } catch {
    return null;
  }
}

/** The system preference; null where the browser cannot tell (tests). */
function darkQuery(): MediaQueryList | null {
  return typeof window.matchMedia === "function"
    ? window.matchMedia(DARK_QUERY)
    : null;
}

function systemTheme(): Theme {
  return darkQuery()?.matches ? "dark" : "light";
}

/**
 * The page theme. It follows the system until the user picks one with
 * `toggle`; the pick is saved in the browser.
 */
export function useTheme(): { theme: Theme; toggle: () => void } {
  const [choice, setChoice] = useState<Theme | null>(readChoice);
  const [system, setSystem] = useState<Theme>(systemTheme);
  const theme = choice ?? system;

  useEffect(() => {
    const query = darkQuery();
    if (!query) return undefined;
    const update = () => {
      setSystem(query.matches ? "dark" : "light");
    };
    query.addEventListener("change", update);
    return () => {
      query.removeEventListener("change", update);
    };
  }, []);

  // Before paint, so a saved dark theme never flashes light.
  useLayoutEffect(() => {
    document.documentElement.classList.toggle(DARK_CLASS, theme === "dark");
  }, [theme]);

  const toggle = useCallback(() => {
    const next: Theme = theme === "dark" ? "light" : "dark";
    setChoice(next);
    try {
      window.localStorage.setItem(THEME_KEY, next);
    } catch {
      // The theme is a convenience; blocked storage keeps it for this page.
    }
  }, [theme]);

  return { theme, toggle };
}
