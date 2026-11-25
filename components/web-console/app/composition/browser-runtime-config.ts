// Reads the allowlisted runtime configuration the BFF injects into the served
// HTML as a <meta> tag. Keeping this out of an inline script means the SPA needs
// no script-src hash to learn operator settings such as the trace sample ratio.

const runtimeConfigMetaName = "hypershell-runtime-config";

export interface BrowserRuntimeConfig {
  /** API server build version, relayed by the BFF from API metadata. */
  apiVersion: string;
  /**
   * Optional `hsctl` release download directory for the Install `hsctl` page,
   * relayed from the BFF `HSCTL_DOWNLOAD_BASE_URL` setting as
   * `hsctl_download_base_url` (WEB-UDOC-08).
   */
  hsctlDownloadBaseUrl?: string;
  tracing: {
    /** Fraction of browser-rooted traces to record, 0..1. */
    sampleRatio: number;
  };
  /** Web console build version, stamped into the image at build time. */
  webVersion: string;
  /** Web console build time, stamped into the image at build time. */
  webBuildTime: string;
}

// When no config is available the browser records nothing: it must never emit
// traces the BFF cannot relay (a dev server or a deployment with tracing off).
const disabledRuntimeConfig: BrowserRuntimeConfig = {
  apiVersion: "unknown",
  tracing: { sampleRatio: 0 },
  webVersion: "unknown",
  webBuildTime: "unknown",
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

// The BFF validates the setting at startup; the browser checks it again so a
// tampered surface can never put a non-HTTPS or query-carrying URL into a
// copyable install command.
function downloadBaseUrlOrUndefined(value: unknown): string | undefined {
  if (typeof value !== "string" || value === "") {
    return undefined;
  }
  try {
    const url = new URL(value);
    return url.protocol === "https:" &&
      !url.username &&
      !url.password &&
      !url.search &&
      !url.hash &&
      !value.includes("?") &&
      !value.includes("#")
      ? value.replace(/\/+$/u, "")
      : undefined;
  } catch {
    return undefined;
  }
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
      hsctl_download_base_url?: unknown;
      tracing?: { sampleRatio?: unknown };
      webVersion?: unknown;
      webBuildTime?: unknown;
    };
    const hsctlDownloadBaseUrl = downloadBaseUrlOrUndefined(
      parsed.hsctl_download_base_url,
    );
    const download =
      hsctlDownloadBaseUrl === undefined ? {} : { hsctlDownloadBaseUrl };
    const sampleRatio = parsed.tracing?.sampleRatio;
    if (!isSampleRatio(sampleRatio)) {
      return { ...disabledRuntimeConfig, ...download };
    }
    return {
      apiVersion: versionOrUnknown(parsed.apiVersion),
      ...download,
      tracing: { sampleRatio },
      webVersion: versionOrUnknown(parsed.webVersion),
      webBuildTime: versionOrUnknown(parsed.webBuildTime),
    };
  } catch {
    return disabledRuntimeConfig;
  }
}
