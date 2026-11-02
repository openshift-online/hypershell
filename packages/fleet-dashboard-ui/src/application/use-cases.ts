// Use cases: thin orchestration over the FleetApi port. Each returns the plane
// untouched today, but this is the seam where cross-plane joins or policy would
// live - keeping that logic out of both adapters and components.

import type { AgentSdlcData } from "../domain/agent-sdlc";
import type { E2eRunDetail, E2eRunsData } from "../domain/e2e-run";
import type { FleetData } from "../domain/fleet";
import type { Plane } from "../domain/plane";
import type { PromotionData } from "../domain/promotion";
import type { SdlcAgentsData } from "../domain/sdlc-agents";
import {
  e2eWorkflow,
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

export function getSdlcAgents(
  api: FleetApi,
  signal?: AbortSignal,
): Promise<Plane<SdlcAgentsData>> {
  return api.getSdlcAgents(signal);
}

// getE2eRuns / getE2eRun publish the e2e-load workflow outcome through the
// observability port (E2E-LOG-13): started up front, then succeeded, cancelled (the
// request was aborted), or failed. The port defaults to no-op so callers without
// observability wired behave unchanged.
export async function getE2eRuns(
  api: FleetApi,
  signal?: AbortSignal,
  observability: ObservabilityPort = noopObservability,
): Promise<Plane<E2eRunsData>> {
  observability.publishE2eWorkflow(e2eWorkflow("e2e.runs.load", "started"));
  try {
    const plane = await api.getE2eRuns(signal);
    observability.publishE2eWorkflow(e2eWorkflow("e2e.runs.load", "succeeded"));
    return plane;
  } catch (error) {
    observability.publishE2eWorkflow(
      e2eWorkflow("e2e.runs.load", signal?.aborted ? "cancelled" : "failed"),
    );
    throw error;
  }
}

export async function getE2eRun(
  api: FleetApi,
  id: string,
  signal?: AbortSignal,
  observability: ObservabilityPort = noopObservability,
): Promise<Plane<E2eRunDetail>> {
  observability.publishE2eWorkflow(e2eWorkflow("e2e.run.load", "started"));
  try {
    const plane = await api.getE2eRun(id, signal);
    observability.publishE2eWorkflow(e2eWorkflow("e2e.run.load", "succeeded"));
    return plane;
  } catch (error) {
    observability.publishE2eWorkflow(
      e2eWorkflow("e2e.run.load", signal?.aborted ? "cancelled" : "failed"),
    );
    throw error;
  }
}
