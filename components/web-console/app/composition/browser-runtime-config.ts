// Reads the allowlisted runtime configuration the BFF injects into the served
// HTML as a <meta> tag. Keeping this out of an inline script means the SPA needs
// no script-src hash to learn operator settings such as the trace sample ratio.

const runtimeConfigMetaName = "hypershell-runtime-config";

export interface BrowserRuntimeConfig {
  tracing: {
    /** Fraction of browser-rooted traces to record, 0..1. */
    sampleRatio: number;
  };
}

// When no config is available the browser records nothing: it must never emit
// traces the BFF cannot relay (a dev server or a deployment with tracing off).
const disabledRuntimeConfig: BrowserRuntimeConfig = {
  tracing: { sampleRatio: 0 },
};

function isSampleRatio(value: unknown): value is number {
  return (
    typeof value === "number" &&
    Number.isFinite(value) &&
    value >= 0 &&
    value <= 1
  );
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
      tracing?: { sampleRatio?: unknown };
    };
    const sampleRatio = parsed.tracing?.sampleRatio;
    return isSampleRatio(sampleRatio)
      ? { tracing: { sampleRatio } }
      : disabledRuntimeConfig;
  } catch {
    return disabledRuntimeConfig;
  }
}
