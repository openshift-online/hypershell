/** The API build identity that the user menu needs. */
export interface ApiVersionReader {
  readVersion(signal?: AbortSignal): Promise<string>;
}

const metadataPath = "/api/hypershell/v1/metadata";

function versionFromMetadata(body: unknown): string {
  if (typeof body !== "object" || body === null) {
    throw new Error("API metadata response is not an object");
  }
  const version = (body as { version?: unknown }).version;
  if (typeof version !== "string" || version.trim() === "") {
    throw new Error("API metadata response has no version");
  }
  return version.trim();
}

/** Reads the API image version through the same-origin BFF proxy. */
export function createApiVersionAdapter(
  fetchImplementation: typeof globalThis.fetch = globalThis.fetch,
): ApiVersionReader {
  return {
    async readVersion(signal) {
      const response = await fetchImplementation(metadataPath, {
        credentials: "same-origin",
        headers: { accept: "application/json" },
        ...(signal ? { signal } : {}),
      });
      if (!response.ok) {
        throw new Error(
          `API metadata request failed with ${String(response.status)}`,
        );
      }
      return versionFromMetadata(await response.json());
    },
  };
}
