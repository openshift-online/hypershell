// Application ports. The presentation/adapter layers supply concrete
// implementations; the use cases below depend only on these interfaces so the
// domain stays framework- and transport-free.

import type { AgentSdlcData } from "../domain/agent-sdlc";
import type { SdlcAgentsData } from "../domain/sdlc-agents";
import type {
  E2eLogLine,
  E2eLogPage,
  E2eRunDetail,
  E2eRunEvent,
  E2eRunsData,
  E2eStreamStatus,
} from "../domain/e2e-run";
import type { FleetData } from "../domain/fleet";
import type { Plane } from "../domain/plane";
import type { PromotionData } from "../domain/promotion";

// The topology plane is modelled in the domain (per-hub spoke layout); the wire
// adapter decodes /api/topology into it. Re-exported here so FleetApi's port stays
// expressed in the type the rest of the app already imports from ports.
export type {
  InstanceTopology,
  TopologyData,
  TopologyHub,
  TopologySpoke,
} from "../domain/topology";
import type { TopologyData } from "../domain/topology";

/** One managed instance's summary, as reported by the server. */
export interface InstanceSummary {
  readonly name: string;
  readonly role: string | null;
  readonly provider: string | null;
  readonly region: string | null;
  readonly health: string | null;
}

export interface InstancesData {
  readonly instances: readonly InstanceSummary[];
}

/**
 * Live-stream handlers for one e2e run (E2E-LOG-05/11). Events are forwarded raw so
 * the framework-free domain reduces them; status reports the live/disconnected/gap
 * state. The transport (EventSource / streaming fetch) stays in the adapter.
 */
export interface E2eStreamHandlers {
  onEvent(event: E2eRunEvent): void;
  /**
   * A single live raw log line for an in-progress run (E2E-LOG-21), delivered on
   * the `log` frame. The browser appends it to the same rendered log list it
   * paginates, de-duplicating the history-to-live seam by `sequence`.
   */
  onLog(line: E2eLogLine): void;
  onStatus(status: E2eStreamStatus): void;
}

/** Read side for every plane the dashboard renders. */
export interface FleetApi {
  getFleet(signal?: AbortSignal): Promise<Plane<FleetData>>;
  getPromotion(signal?: AbortSignal): Promise<Plane<PromotionData>>;
  getTopology(signal?: AbortSignal): Promise<Plane<TopologyData>>;
  getInstances(signal?: AbortSignal): Promise<Plane<InstancesData>>;
  getSdlc(signal?: AbortSignal): Promise<Plane<AgentSdlcData>>;
  getSdlcAgents(signal?: AbortSignal): Promise<Plane<SdlcAgentsData>>;
  getE2eRuns(signal?: AbortSignal): Promise<Plane<E2eRunsData>>;
  getE2eRun(id: string, signal?: AbortSignal): Promise<Plane<E2eRunDetail>>;
  /**
   * Read one bounded page of a run's raw-log history (E2E-LOG-19). `before` is the
   * nanosecond cursor of the oldest line already held (omit for the newest page);
   * the page returns older lines plus the next cursor and a hasMore signal. Logs are
   * paged separately from the run detail so a slow or failed log read never blanks
   * the steps.
   *
   * `stepId` scopes the page to a single selected step (E2E-LOG-23): the backend
   * filters by that step in Loki before the page limit and bounds the time range to
   * the step's lifecycle window (E2E-LOG-24/25). Pass `null` for the run-wide
   * history, which behaves exactly as before.
   */
  getE2eRunLogs(
    id: string,
    before: string | null,
    stepId: string | null,
    signal?: AbortSignal,
  ): Promise<Plane<E2eLogPage>>;
  /**
   * Subscribe to a run's live stream. Returns an unsubscribe thunk that cancels the
   * subscription (the only cancellable-subscription precedent in the repo is
   * adapters/auth/session-expiry's subscribe/unsubscribe). The optional signal also
   * cancels it, so React effects can wire either teardown path.
   */
  streamE2eRun(
    id: string,
    handlers: E2eStreamHandlers,
    signal?: AbortSignal,
  ): () => void;
}
