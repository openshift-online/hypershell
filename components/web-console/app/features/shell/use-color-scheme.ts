import { useCallback, useSyncExternalStore } from "react";

const STORAGE_KEY = "hypershell-color-scheme";
const DARK_CLASS = "pf-v6-theme-dark";

type ColorScheme = "light" | "dark";

function getSnapshot(): ColorScheme {
  return document.documentElement.classList.contains(DARK_CLASS)
    ? "dark"
    : "light";
}

function getServerSnapshot(): ColorScheme {
  return "light";
}

function subscribe(onStoreChange: () => void): () => void {
  const observer = new MutationObserver(onStoreChange);
  observer.observe(document.documentElement, {
    attributes: true,
    attributeFilter: ["class"],
  });
  return () => observer.disconnect();
}

function applyScheme(scheme: ColorScheme): void {
  document.documentElement.classList.toggle(DARK_CLASS, scheme === "dark");
}

function readPreference(): ColorScheme {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored === "dark" || stored === "light") return stored;
  } catch {
    /* storage unavailable */
  }
  return window.matchMedia("(prefers-color-scheme: dark)").matches
    ? "dark"
    : "light";
}

let initialized = false;

export function useColorScheme() {
  if (typeof document !== "undefined" && !initialized) {
    initialized = true;
    applyScheme(readPreference());
  }

  const scheme = useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);

  const toggle = useCallback(() => {
    const next: ColorScheme = getSnapshot() === "dark" ? "light" : "dark";
    applyScheme(next);
    try {
      localStorage.setItem(STORAGE_KEY, next);
    } catch {
      /* storage unavailable */
    }
  }, []);

  return { scheme, toggle } as const;
}
