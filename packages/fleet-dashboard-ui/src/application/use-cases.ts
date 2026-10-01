// Use cases: thin orchestration over the FleetApi port. Each returns the plane
// untouched today, but this is the seam where cross-plane joins or policy would
// live - keeping that logic out of both adapters and components.

import type { FleetData } from "../domain/fleet";
import type { Plane } from "../domain/plane";
import type { PromotionData } from "../domain/promotion";
import type { FleetApi, InstancesData, TopologyData } from "./ports";

export function getFleet(
  api: FleetApi,
  signal?: AbortSignal,
): Promise<Plane<FleetData>> {
  return api.getFleet(signal);
}

export function getPromotion(
  api: FleetApi,
  signal?: AbortSignal,
): Promise<Plane<PromotionData>> {
  return api.getPromotion(signal);
}

export function getTopology(
  api: FleetApi,
  signal?: AbortSignal,
): Promise<Plane<TopologyData>> {
  return api.getTopology(signal);
}

export function getInstances(
  api: FleetApi,
  signal?: AbortSignal,
): Promise<Plane<InstancesData>> {
  return api.getInstances(signal);
}
