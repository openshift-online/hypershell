// React context carrying the observability port. Defaults to the no-op port so a
// component can publish facts without a provider wired (production can supply a
// fan-out port with real sinks later, with no change to publishing code).

import { createContext, useContext } from "react";

import {
  noopObservability,
  type ObservabilityPort,
} from "../../application/observability";

const ObservabilityContext =
  createContext<ObservabilityPort>(noopObservability);

export const ObservabilityProvider = ObservabilityContext.Provider;

export function useObservability(): ObservabilityPort {
  return useContext(ObservabilityContext);
}
