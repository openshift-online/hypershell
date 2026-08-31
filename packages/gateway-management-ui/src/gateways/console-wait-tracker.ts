import { useCallback, useRef } from "react";

import { resolveConsoleWaitStart } from "./gateway-data";
import type { GatewayRecord } from "../application/gateway-types";

type ConsoleWaitGateway = Pick<
  GatewayRecord,
  "phase" | "status" | "externalDns" | "consoleUrl" | "gatewayVersion"
>;

// Records the returned type of useConsoleWaitTracker: a stable function that,
// given a gateway id and its current record, returns the timestamp from which
// that gateway's connection-data polling deadline should be measured. It returns
// undefined when the gateway does not wait for a console address or version.
export type ConsoleWaitTracker = (
  gatewayId: string,
  gateway: ConsoleWaitGateway,
  now?: number,
) => number | undefined;

// Tracks when the UI first observes that each gateway needs a console address or
// runtime version. The map is local to the component. A new visit starts a new
// bounded wait.
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
