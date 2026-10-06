// Application ports. The presentation/adapter layers supply concrete
// implementations; the use cases below depend only on these interfaces so the
// domain stays framework- and transport-free.

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

/** Read side for every plane the dashboard renders. */
export interface FleetApi {
  getFleet(signal?: AbortSignal): Promise<Plane<FleetData>>;
  getPromotion(signal?: AbortSignal): Promise<Plane<PromotionData>>;
  getTopology(signal?: AbortSignal): Promise<Plane<TopologyData>>;
  getInstances(signal?: AbortSignal): Promise<Plane<InstancesData>>;
}
