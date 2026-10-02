// Application ports. The presentation/adapter layers supply concrete
// implementations; the use cases below depend only on these interfaces so the
// domain stays framework- and transport-free.

import type { FleetData } from "../domain/fleet";
import type { Plane } from "../domain/plane";
import type { PromotionData } from "../domain/promotion";

/** Topology is opaque to the domain - the server owns its shape entirely. */
export interface TopologyData {
  readonly nodes: readonly TopologyNode[];
  readonly edges: readonly TopologyEdge[];
}

export interface TopologyNode {
  readonly id: string;
  readonly label: string;
  readonly kind: string;
}

export interface TopologyEdge {
  readonly from: string;
  readonly to: string;
}

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
