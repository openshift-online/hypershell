import { useCallback, useRef } from "react";

import { resolveConsoleWaitStart } from "./gateway-data";
import type { GatewayRecord } from "../application/gateway-types";

type ConsoleWaitGateway = Pick<
  GatewayRecord,
  "phase" | "status" | "externalDns" | "consoleUrl"
>;

// Records the returned type of useConsoleWaitTracker: a stable function that,
// given a gateway id and its current record, returns the timestamp from which
// that gateway's console-ready polling deadline should be measured (or undefined
// when it is not awaiting a console).
export type ConsoleWaitTracker = (
  gatewayId: string,
  gateway: ConsoleWaitGateway,
  now?: number,
) => number | undefined;

// Tracks, per gateway id, when the UI first observed the gateway awaiting its
// console, so the console-ready polling deadline is measured from that moment
// rather than the gateway's creation time. Anchoring on createdAt wrongly marks a
// pre-existing routed gateway (created long ago) or one first routed well after
// creation as past the deadline the instant it loads, so it would never poll for
// its console address. The map is component-local: navigating away and back
// restarts the clock, which is acceptable and mirrors the control plane's
// in-memory route-readiness window (see health.go routeNotReadySince).
export function useConsoleWaitTracker(): ConsoleWaitTracker {
  const startsRef = useRef<Map<string, number>>(new Map());
  return useCallback((gatewayId, gateway, now = Date.now()) => {
    const starts = startsRef.current;
    const next = resolveConsoleWaitStart(gateway, now, starts.get(gatewayId));
    if (next === undefined) {
      starts.delete(gatewayId);
    } else {
      starts.set(gatewayId, next);
    }
    return next;
  }, []);
}
