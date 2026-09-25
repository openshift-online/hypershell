// Reads the allowlisted runtime configuration the BFF injects into the served
// HTML as a <meta> tag. Keeping this out of an inline script means the SPA needs
// no script-src hash to learn operator settings such as the trace sample ratio.

const runtimeConfigMetaName = "hypershell-runtime-config";

export interface BrowserRuntimeConfig {
  /** API server build version, relayed by the BFF from API metadata. */
  apiVersion: string;
  tracing: {
    /** Fraction of browser-rooted traces to record, 0..1. */
    sampleRatio: number;
  };
  /** Web console build version, stamped into the image at build time. */
  webVersion: string;
}

// When no config is available the browser records nothing: it must never emit
// traces the BFF cannot relay (a dev server or a deployment with tracing off).
const disabledRuntimeConfig: BrowserRuntimeConfig = {
  apiVersion: "unknown",
  tracing: { sampleRatio: 0 },
  webVersion: "unknown",
};

function isSampleRatio(value: unknown): value is number {
  return (
    typeof value === "number" &&
    Number.isFinite(value) &&
    value >= 0 &&
    value <= 1
  );
}

// Versions are display-only facts, so a non-string or missing value degrades
// to "unknown" without disabling tracing.
function versionOrUnknown(value: unknown): string {
  return typeof value === "string" && value.length > 0 ? value : "unknown";
}

/**
 * Parses the runtime config from the injected <meta> tag. An absent tag,
 * unparsable content, or an out-of-range sample ratio all fall back to the
 * disabled config, so a missing or tampered surface fails closed to no tracing
 * rather than defaulting to recording every trace. On the server there is no
 * document, so the disabled config is returned and no browser tracing starts.
 */
export function readBrowserRuntimeConfig(
  doc: Document | undefined = typeof document === "undefined"
    ? undefined
    : document,
): BrowserRuntimeConfig {
  const content = doc
    ?.querySelector(`meta[name="${runtimeConfigMetaName}"]`)
    ?.getAttribute("content");
  if (content === null || content === undefined) {
    return disabledRuntimeConfig;
  }
  try {
    const parsed = JSON.parse(content) as {
      apiVersion?: unknown;
      tracing?: { sampleRatio?: unknown };
      webVersion?: unknown;
    };
    const sampleRatio = parsed.tracing?.sampleRatio;
    if (!isSampleRatio(sampleRatio)) {
      return disabledRuntimeConfig;
    }
    return {
      apiVersion: versionOrUnknown(parsed.apiVersion),
      tracing: { sampleRatio },
      webVersion: versionOrUnknown(parsed.webVersion),
    };
  } catch {
    return disabledRuntimeConfig;
  }
}
