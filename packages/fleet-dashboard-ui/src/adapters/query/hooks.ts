// TanStack Query hooks - the only place server state is read. No global polling:
// each plane refetches on a bounded interval and TanStack pauses refetching when
// the tab is hidden (refetchIntervalInBackground defaults to false).

import { useQuery, type UseQueryResult } from "@tanstack/react-query";

import type { FleetData } from "../../domain/fleet";
import type { Plane } from "../../domain/plane";
import type { PromotionData } from "../../domain/promotion";
import type { InstancesData, TopologyData } from "../../application/ports";
import {
  getFleet,
  getInstances,
  getPromotion,
  getTopology,
} from "../../application/use-cases";
import { useFleetApi } from "./api-context";
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
