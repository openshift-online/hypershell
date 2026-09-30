// HTTP adapter implementing the FleetApi port against the Go BFF's /api/* routes.
// The base path is injected (never hard-coded to a host); everything about the
// fleet's identity comes back inside the JSON payloads, not the URLs.

import type { FleetData } from "../../domain/fleet";
import type { Plane } from "../../domain/plane";
import type { PromotionData } from "../../domain/promotion";
import type {
  FleetApi,
  InstancesData,
  TopologyData,
} from "../../application/ports";

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

  async function getPlane<T>(
    path: string,
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
    return (await response.json()) as Plane<T>;
  }

  return {
    getFleet: (signal) => getPlane<FleetData>("/fleet", signal),
    getPromotion: (signal) => getPlane<PromotionData>("/promotion", signal),
    getTopology: (signal) => getPlane<TopologyData>("/topology", signal),
    getInstances: (signal) => getPlane<InstancesData>("/instances", signal),
  };
}
