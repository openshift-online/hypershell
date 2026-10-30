import type { SpecSourcePort } from "../application/ports";
import type { SpecFileSource } from "../domain/spec-document";

// Relative so the build works under any base path, such as /specs/ in the
// fleet dashboard.
export const SPECS_ENDPOINT = "api/specs.json";
export const SPECS_CHANGED_EVENT = "spec-viewer:specs-changed";

/** Adds `ref` to an API URL; "" leaves the server's default. */
export function withRef(url: string, ref: string): string {
  if (ref === "") return url;
  return `${url}${url.includes("?") ? "&" : "?"}ref=${encodeURIComponent(ref)}`;
}

function isSpecFileSource(value: unknown): value is SpecFileSource {
  if (typeof value !== "object" || value === null) return false;
  const record = value as Record<string, unknown>;
  return typeof record.path === "string" && typeof record.content === "string";
}

export const httpSpecSource: SpecSourcePort = {
  async loadSpecFiles(ref) {
    const response = await fetch(withRef(SPECS_ENDPOINT, ref));
    if (!response.ok) {
      throw new Error(
        `Loading specs failed: HTTP ${String(response.status)} ${await response.text()}`,
      );
    }
    const body: unknown = await response.json();
    if (!Array.isArray(body) || !body.every(isSpecFileSource)) {
      throw new Error("Loading specs failed: unexpected response shape");
    }
    return body;
  },
  subscribe(onChange) {
    const hot = import.meta.hot;
    if (!hot) return () => undefined;
    hot.on(SPECS_CHANGED_EVENT, onChange);
    return () => {
      hot.off(SPECS_CHANGED_EVENT, onChange);
    };
  },
};
