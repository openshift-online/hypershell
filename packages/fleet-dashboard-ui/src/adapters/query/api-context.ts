// React context carrying the FleetApi port. The composition root provides the
// concrete HTTP adapter; tests provide a fake. Hooks never construct an adapter.

import { createContext, useContext } from "react";

import type { FleetApi } from "../../application/ports";

const FleetApiContext = createContext<FleetApi | null>(null);

export const FleetApiProvider = FleetApiContext.Provider;

export function useFleetApi(): FleetApi {
  const api = useContext(FleetApiContext);
  if (api === null) {
    throw new Error("useFleetApi must be used within a FleetApiProvider");
  }
  return api;
}
