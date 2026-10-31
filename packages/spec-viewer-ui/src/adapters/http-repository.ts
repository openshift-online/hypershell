import type { RefList, RepositoryPort } from "../application/ports";
import type { FileReferences } from "../domain/references";
import { parseMarkerLines } from "../domain/traceability";
import { withRef } from "./http-spec-source";

async function fetchJson(url: string, what: string): Promise<unknown> {
  let response: Response;
  try {
    response = await fetch(url);
  } catch (error) {
    throw new Error(
      `${what} failed: ${error instanceof Error ? error.message : String(error)}`,
    );
  }
  if (!response.ok) {
    // The server explains the failure, such as a missing repository checkout.
    throw new Error(
      `${what} failed: HTTP ${String(response.status)} ${await response.text()}`,
    );
  }
  return response.json();
}

function isFileReferences(value: unknown): value is FileReferences {
  if (typeof value !== "object" || value === null) return false;
  const record = value as Record<string, unknown>;
  return (
    typeof record.path === "string" &&
    typeof record.isTest === "boolean" &&
    Array.isArray(record.matches)
  );
}

function isRefList(value: unknown): value is RefList {
  if (typeof value !== "object" || value === null) return false;
  const record = value as Record<string, unknown>;
  return (
    typeof record.default === "string" &&
    Array.isArray(record.refs) &&
    record.refs.every((ref) => typeof ref === "string")
  );
}

export const httpRepository: RepositoryPort = {
  async listRefs() {
    const body = await fetchJson("api/refs", "Listing branches");
    if (!isRefList(body)) {
      throw new Error("Listing branches failed: unexpected response shape");
    }
    return body;
  },
  async findReferences(ref, id) {
    const body = await fetchJson(
      withRef(`api/references?id=${encodeURIComponent(id)}`, ref),
      `Searching for ${id}`,
    );
    if (!Array.isArray(body) || !body.every(isFileReferences)) {
      throw new Error(`Searching for ${id} failed: unexpected response shape`);
    }
    return body;
  },
  async loadMarkers(ref) {
    return parseMarkerLines(
      await fetchJson(withRef("api/markers", ref), "Loading test markers"),
    );
  },
  async readFile(ref, path) {
    const body = await fetchJson(
      withRef(`api/file?path=${encodeURIComponent(path)}`, ref),
      `Reading ${path}`,
    );
    const content =
      typeof body === "object" && body !== null
        ? (body as Record<string, unknown>).content
        : undefined;
    if (typeof content !== "string") {
      throw new Error(`Reading ${path} failed: unexpected response shape`);
    }
    return content;
  },
};
