// Locale resolution for IntlProvider. Messages are the built-in defaults today;
// translated catalogs load here when they exist.

export const DEFAULT_LOCALE = "en";

export function resolveLocale(): string {
  // Presentation-layer concern: read the browser's preference, fall back to en.
  const preferred = globalThis.navigator.language;
  return preferred || DEFAULT_LOCALE;
}
