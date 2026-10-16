// Use cases: thin orchestration over the FleetApi port. Each returns the plane
// untouched today, but this is the seam where cross-plane joins or policy would
// live - keeping that logic out of both adapters and components.

import type { AgentSdlcData } from "../domain/agent-sdlc";
import type { FleetData } from "../domain/fleet";
import type { Plane } from "../domain/plane";
import type { PromotionData } from "../domain/promotion";
import {
  noopObservability,
  sdlcWorkflow,
  type ObservabilityPort,
} from "./observability";
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

// getSdlc publishes the SDLC-load workflow outcome through the observability port
// (SDLC-VIS-15): started up front, then succeeded, cancelled (the request was
// aborted), or failed. The port defaults to no-op so callers without observability
// wired behave unchanged.
export async function getSdlc(
  api: FleetApi,
  signal?: AbortSignal,
  observability: ObservabilityPort = noopObservability,
): Promise<Plane<AgentSdlcData>> {
  observability.publishSdlcWorkflow(sdlcWorkflow("started"));
  try {
    const plane = await api.getSdlc(signal);
    observability.publishSdlcWorkflow(sdlcWorkflow("succeeded"));
    return plane;
  } catch (error) {
    observability.publishSdlcWorkflow(
      sdlcWorkflow(signal?.aborted ? "cancelled" : "failed"),
    );
    throw error;
  }
}
