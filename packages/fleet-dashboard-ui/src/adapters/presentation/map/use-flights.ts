// Drives the promotion fly-in overlay: watches the map model across polls, and when
// computeFlights() reports a bundle landing on a downstream node, registers a transient
// "flight" that the MapFlights overlay animates source -> dest. Each flight self-expires
// after the arc + dust have played, and its landing bumps a screen-shake nonce so the
// whole canvas can jolt when the bundle slams down. Honours prefers-reduced-motion by
// not launching flights at all (the map just updates in place).

import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";

import { computeFlights, type Flight } from "../../../domain/map/flights";
import type { MapModel } from "../../../domain/map/model";

/** Arc travel time; must stay in sync with the overlay's .flightTravel/.flightScale dur. */
export const FLIGHT_MS = 900;
/** Dust + impact ring linger past landing before the flying copy is removed (their CSS
 *  delay ~matches FLIGHT_MS, then they play ~0.65s). */
const DUST_MS = 800;

export interface ActiveFlight extends Flight {
  /** Stable React key so re-renders never restart an in-flight animation. */
  readonly key: string;
}

let seq = 0;

function prefersReducedMotion(): boolean {
  return (
    typeof window !== "undefined" &&
    typeof window.matchMedia === "function" &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches
  );
}

export interface FlightsState {
  readonly flights: readonly ActiveFlight[];
  /** destId -> the seed its card should KEEP showing while a flight is inbound, so the
   *  landing node doesn't switch icons before the flying copy arrives. Cleared per dest
   *  when its flight is removed (the copy has landed), revealing the new bundle. */
  readonly holdSeedById: ReadonlyMap<string, string>;
}

export function useFlights(model: MapModel): FlightsState {
  const prevRef = useRef<MapModel | null>(null);
  const [flights, setFlights] = useState<readonly ActiveFlight[]>([]);
  // Pending flight-cull timers, held in a ref so they survive model re-renders.
  const timers = useRef<Set<ReturnType<typeof setTimeout>>>(new Set());

  // Clear timers ONLY on unmount - never on each model change. The map model is a
  // fresh object every poll (fleet refetches even when seeds are unchanged), so tying
  // the cull timer to the [model] effect's cleanup meant an unrelated fleet poll landing
  // mid-flight would clearTimeout the pending removal, and the next effect run (no new
  // flights) never rescheduled it - stranding the frozen flying copy on the card
  // forever (compounding drop-shadows; the stale copy read as the bundle "reverting").
  useEffect(() => {
    const pending = timers.current;
    return () => {
      pending.forEach((t) => {
        clearTimeout(t);
      });
      pending.clear();
    };
  }, []);

  // useLayoutEffect (not useEffect): registering a flight also starts the dest's icon
  // "hold" (via holdSeedById below). Running before paint means the hold is committed in
  // the SAME frame the new model arrives, so the landing card never flashes the new icon
  // for a frame before the hold kicks in - it shows the old icon until the copy lands.
  useLayoutEffect(() => {
    const prev = prevRef.current;
    prevRef.current = model;
    if (!prev || prefersReducedMotion()) return;

    const found = computeFlights(prev, model);
    if (found.length === 0) return;

    const active = found.map((f) => ({ ...f, key: `flight-${String(seq++)}` }));
    const keys = new Set(active.map((a) => a.key));
    setFlights((cur) => [...cur, ...active]);

    // Each flight owns its lifecycle via a self-removing timer (tracked for unmount
    // cleanup). This effect intentionally returns NO cleanup, so a later model change
    // can't cancel an in-flight flight's cull. State is set only from this async
    // callback, off the render-cascade path (react-hooks/set-state-in-effect).
    const track = timers.current;
    const id = window.setTimeout(() => {
      track.delete(id);
      setFlights((cur) => cur.filter((a) => !keys.has(a.key)));
    }, FLIGHT_MS + DUST_MS);
    track.add(id);
  }, [model]);

  // The seed each landing node should keep showing until its flight's copy arrives. One
  // flight per dest per transition (steps are far wider than a flight), so last wins.
  const holdSeedById = useMemo(() => {
    const m = new Map<string, string>();
    for (const fl of flights) m.set(fl.destId, fl.destPrevSeed);
    return m;
  }, [flights]);

  return { flights, holdSeedById };
}
