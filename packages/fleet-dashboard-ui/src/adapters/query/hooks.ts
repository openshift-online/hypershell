// TanStack Query hooks - the only place server state is read. No global polling:
// each plane refetches on a bounded interval and TanStack pauses refetching when
// the tab is hidden (refetchIntervalInBackground defaults to false).

import { useQuery, type UseQueryResult } from "@tanstack/react-query";

import type { AgentSdlcData } from "../../domain/agent-sdlc";
import type { DeliveryData } from "../../domain/delivery";
import type { SdlcAgentsData } from "../../domain/sdlc-agents";
import type {
  E2eDiagnosticsData,
  E2eRunDetail,
  E2eRunsData,
} from "../../domain/e2e-run";
import type { FleetData } from "../../domain/fleet";
import type { Plane } from "../../domain/plane";
import type { PromotionData } from "../../domain/promotion";
import type { InstancesData, TopologyData } from "../../application/ports";
import {
  getDelivery,
  getE2eDiagnostics,
  getE2eRun,
  getE2eRuns,
  getFleet,
  getInstances,
  getPromotion,
  getSdlc,
  getSdlcAgents,
  getTopology,
} from "../../application/use-cases";
import { useFleetApi } from "./api-context";
import { useObservability } from "../observability/observability-context";
import { queryKeys } from "./keys";

// DEV-only fast cadence: the local mock server can set VITE_FAST_POLL_MS to speed up
// the fleet + promotion refetch so value-change animations can be watched without
// waiting out the real intervals. Parsed to a positive number or ignored; unset in
// normal and production builds, where the cadence below applies unchanged.
const fastPollRaw = Number(import.meta.env.VITE_FAST_POLL_MS);
const FAST_POLL_MS =
  Number.isFinite(fastPollRaw) && fastPollRaw > 0 ? fastPollRaw : null;

/** Per-plane refetch cadence (ms). Fleet/promotion move fastest; topology rarely. */
const REFETCH_MS = {
  fleet: FAST_POLL_MS ?? 15_000,
  promotion: FAST_POLL_MS ?? 20_000,
  instances: 30_000,
  topology: 60_000,
  // SDLC reconstructs label history from the GitHub timeline on the BFF's own
  // bounded refresh loop; the UI only polls the cached plane, so a relaxed cadence
  // keeps the board "near real time" without adding client pressure (SDLC-VIS-04).
  sdlc: FAST_POLL_MS ?? 30_000,
  sdlcAgents: FAST_POLL_MS ?? 30_000,
  // The recent-runs list is a bounded Loki query_range snapshot; live progress
  // arrives on the per-run stream (E2E-LOG-05), so the list polls at a relaxed
  // cadence just to pick up newly-started or newly-finished runs.
  e2eRuns: FAST_POLL_MS ?? 30_000,
  // A single run's snapshot detail is the stream's backfill baseline; a relaxed
  // poll reconciles any drift without competing with the live stream.
  e2eRun: FAST_POLL_MS ?? 30_000,
  // The BFF refreshes the delivery plane on its own interval (FD_REFRESH_DELIVERY,
  // 60s by default); the UI polls the cached plane a little faster.
  delivery: FAST_POLL_MS ?? 30_000,
};

export function useFleet(): UseQueryResult<Plane<FleetData>> {
  const api = useFleetApi();
  return useQuery({
    queryKey: queryKeys.fleet,
    queryFn: ({ signal }) => getFleet(api, signal),
    refetchInterval: REFETCH_MS.fleet,
  });
}

export function usePromotion(): UseQueryResult<Plane<PromotionData>> {
  const api = useFleetApi();
  return useQuery({
    queryKey: queryKeys.promotion,
    queryFn: ({ signal }) => getPromotion(api, signal),
    refetchInterval: REFETCH_MS.promotion,
  });
}

export function useTopology(): UseQueryResult<Plane<TopologyData>> {
  const api = useFleetApi();
  return useQuery({
    queryKey: queryKeys.topology,
    queryFn: ({ signal }) => getTopology(api, signal),
    refetchInterval: REFETCH_MS.topology,
  });
}

export function useInstances(): UseQueryResult<Plane<InstancesData>> {
  const api = useFleetApi();
  return useQuery({
    queryKey: queryKeys.instances,
    queryFn: ({ signal }) => getInstances(api, signal),
    refetchInterval: REFETCH_MS.instances,
  });
}

export function useSdlc(): UseQueryResult<Plane<AgentSdlcData>> {
  const api = useFleetApi();
  const observability = useObservability();
  return useQuery({
    queryKey: queryKeys.sdlc,
    queryFn: ({ signal }) => getSdlc(api, signal, observability),
    refetchInterval: REFETCH_MS.sdlc,
  });
}

export function useE2eRuns(): UseQueryResult<Plane<E2eRunsData>> {
  const api = useFleetApi();
  const observability = useObservability();
  return useQuery({
    queryKey: queryKeys.e2eRuns,
    queryFn: ({ signal }) => getE2eRuns(api, signal, observability),
    refetchInterval: REFETCH_MS.e2eRuns,
  });
}

export function useDelivery(): UseQueryResult<Plane<DeliveryData>> {
  const api = useFleetApi();
  return useQuery({
    queryKey: queryKeys.delivery,
    queryFn: ({ signal }) => getDelivery(api, signal),
    refetchInterval: REFETCH_MS.delivery,
  });
}

export function useSdlcAgents(): UseQueryResult<Plane<SdlcAgentsData>> {
  const api = useFleetApi();
  return useQuery({
    queryKey: queryKeys.sdlcAgents,
    queryFn: ({ signal }) => getSdlcAgents(api, signal),
    refetchInterval: REFETCH_MS.sdlcAgents,
  });
}

export function useE2eRun(
  id: string | null,
): UseQueryResult<Plane<E2eRunDetail>> {
  const api = useFleetApi();
  const observability = useObservability();
  return useQuery({
    queryKey: queryKeys.e2eRun(id ?? ""),
    queryFn: ({ signal }) => getE2eRun(api, id ?? "", signal, observability),
    // Only fetch a detail once a run is selected.
    enabled: id !== null,
    refetchInterval: REFETCH_MS.e2eRun,
  });
}

export function useE2eDiagnostics(
  id: string | null,
  expanded: boolean,
): UseQueryResult<Plane<E2eDiagnosticsData>> {
  const api = useFleetApi();
  const observability = useObservability();
  return useQuery({
    queryKey: queryKeys.e2eDiagnostics(id ?? ""),
    queryFn: ({ signal }) =>
      getE2eDiagnostics(api, id ?? "", signal, observability),
    // Only fetch diagnostics once a run is selected and the section is expanded.
    enabled: id !== null && expanded,
    staleTime: 30_000,
    refetchInterval: expanded ? 30_000 : false,
    retry: false,
  });
}
