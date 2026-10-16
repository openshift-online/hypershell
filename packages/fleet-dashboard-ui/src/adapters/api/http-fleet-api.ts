// HTTP adapter implementing the FleetApi port against the Go BFF's /api/* routes.
// The base path is injected (never hard-coded to a host); everything about the
// fleet's identity comes back inside the JSON payloads, not the URLs.

import type { Plane } from "../../domain/plane";
import type { E2eStreamHandlers, FleetApi } from "../../application/ports";
import {
  mapE2eRunDetail,
  mapE2eRuns,
  mapE2eStreamEvent,
  mapE2eStreamStatus,
  mapFleet,
  mapInstances,
  mapPromotion,
  mapSdlc,
  mapTopology,
} from "./wire";

export interface HttpFleetApiOptions {
  /** BFF base path, e.g. "/api". No trailing slash. */
  readonly basePath: string;
  /** Injected for testability; defaults to the global fetch. */
  readonly fetchImpl?: typeof fetch;
  /** Injected for testability; defaults to the global EventSource (SSE transport). */
  readonly eventSourceImpl?: typeof EventSource;
}

/** One decoded SSE frame (mirrors the BFF streamFrame). */
interface StreamFrame {
  readonly kind?: string;
  readonly event?: unknown;
  readonly status?: unknown;
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

  // The run id is the multi-segment address "tenant/namespace/runId"; URL-encode
  // each segment so a slash in the run id itself cannot alter the path structure.
  const encodeRunId = (id: string): string =>
    id.split("/").map(encodeURIComponent).join("/");

  const eventSourceImpl = options.eventSourceImpl ?? globalThis.EventSource;

  function streamE2eRun(
    id: string,
    handlers: E2eStreamHandlers,
    signal?: AbortSignal,
  ): () => void {
    // The run id goes as a single query value (encoded whole), so the server parses
    // the collision-free address it already knows how to split.
    const url = `${base}/e2e/stream?run=${encodeURIComponent(id)}`;
    // withCredentials so the oauth-proxy session cookie rides along; a long-lived
    // stream is terminated by the auth middleware when authorization expires
    // (E2E-LOG-08), which surfaces here as an error -> disconnected status.
    const es = new eventSourceImpl(url, { withCredentials: true });

    let closed = false;
    const close = (): void => {
      if (closed) {
        return;
      }
      closed = true;
      es.close();
      signal?.removeEventListener("abort", close);
    };

    es.onmessage = (ev: MessageEvent) => {
      let frame: StreamFrame;
      try {
        frame = JSON.parse(ev.data as string) as StreamFrame;
      } catch {
        // A malformed frame is isolated; the rest of the stream continues (E2E-LOG-07).
        return;
      }
      if (frame.kind === "event") {
        const event = mapE2eStreamEvent(frame.event);
        if (event !== null) {
          handlers.onEvent(event);
        }
      } else if (frame.kind === "status") {
        handlers.onStatus(mapE2eStreamStatus(frame.status));
      }
      // kind === "heartbeat" (or anything else): no payload to deliver.
    };
    es.onerror = () => {
      // The browser EventSource reconnects on its own; until it does, the feed is
      // not healthy, so report disconnected rather than leave it reading as live
      // (E2E-LOG-06). The server re-emits a "live" status on reconnect.
      handlers.onStatus({ state: "disconnected", droppedEntries: 0 });
    };

    if (signal) {
      if (signal.aborted) {
        close();
      } else {
        signal.addEventListener("abort", close);
      }
    }
    return close;
  }

  return {
    getFleet: (signal) => getPlane("/fleet", mapFleet, signal),
    getPromotion: (signal) => getPlane("/promotion", mapPromotion, signal),
    getTopology: (signal) => getPlane("/topology", mapTopology, signal),
    getInstances: (signal) => getPlane("/instances", mapInstances, signal),
    getSdlc: (signal) => getPlane("/sdlc", mapSdlc, signal),
    getE2eRuns: (signal) => getPlane("/e2e/runs", mapE2eRuns, signal),
    getE2eRun: (id, signal) =>
      getPlane(`/e2e/runs/${encodeRunId(id)}`, mapE2eRunDetail, signal),
    streamE2eRun,
  };
}
