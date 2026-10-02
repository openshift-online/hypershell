// HTTP adapter implementing the FleetApi port against the Go BFF's /api/* routes.
// The base path is injected (never hard-coded to a host); everything about the
// fleet's identity comes back inside the JSON payloads, not the URLs.

import type { Plane } from "../../domain/plane";
import type { FleetApi, TopologyData } from "../../application/ports";
import { mapFleet, mapInstances, mapPromotion } from "./wire";

export interface HttpFleetApiOptions {
  /** BFF base path, e.g. "/api". No trailing slash. */
  readonly basePath: string;
  /** Injected for testability; defaults to the global fetch. */
  readonly fetchImpl?: typeof fetch;
}

export class HttpError extends Error {
  constructor(
    readonly status: number,
    readonly url: string,
  ) {
    super(`Request to ${url} failed with status ${String(status)}`);
    this.name = "HttpError";
  }
}

export function createHttpFleetApi(options: HttpFleetApiOptions): FleetApi {
  const fetchImpl = options.fetchImpl ?? globalThis.fetch.bind(globalThis);
  const base = options.basePath.replace(/\/$/, "");

  // getPlane fetches an envelope and maps its `data` from the wire shape into the
  // domain shape. The envelope fields (generatedAt/stale/error) are identical on
  // both sides; only `data` needs projecting (see ./wire).
  async function getPlane<T>(
    path: string,
    mapData: (raw: unknown) => T,
    signal?: AbortSignal,
  ): Promise<Plane<T>> {
    const url = `${base}${path}`;
    const response = await fetchImpl(url, {
      signal,
      headers: { Accept: "application/json" },
    });
    if (!response.ok) {
      throw new HttpError(response.status, url);
    }
    const envelope = (await response.json()) as Plane<unknown>;
    return { ...envelope, data: mapData(envelope.data) };
  }

  // Topology is not yet surfaced in the UI; it passes through untouched until its
  // view (and mapper) exists.
  return {
    getFleet: (signal) => getPlane("/fleet", mapFleet, signal),
    getPromotion: (signal) => getPlane("/promotion", mapPromotion, signal),
    getTopology: (signal) =>
      getPlane<TopologyData>("/topology", (raw) => raw as TopologyData, signal),
    getInstances: (signal) => getPlane("/instances", mapInstances, signal),
  };
}
