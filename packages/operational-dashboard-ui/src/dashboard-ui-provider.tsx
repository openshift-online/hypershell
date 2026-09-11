import { createContext, type PropsWithChildren, useContext } from "react";

import type { DashboardOperations } from "./application/dashboard-types";
import type { DashboardProbePublisher } from "./application/dashboard-probes";

export interface DashboardUiNavigation {
  collectionHref: string;
  navigate: (href: string) => Promise<void> | void;
}

export interface DashboardUiServices {
  dashboard: DashboardOperations;
  navigation: DashboardUiNavigation;
  probes?: DashboardProbePublisher;
}

const DashboardUiContext = createContext<DashboardUiServices | undefined>(
  undefined,
);

export function DashboardUiProvider({
  children,
  dashboard,
  navigation,
  probes,
}: PropsWithChildren<DashboardUiServices>) {
  return (
    <DashboardUiContext.Provider value={{ dashboard, navigation, probes }}>
      {children}
    </DashboardUiContext.Provider>
  );
}

export function useDashboardUi(): DashboardUiServices {
  const services = useContext(DashboardUiContext);
  if (!services) {
    throw new Error("Dashboard UI must be rendered within DashboardUiProvider");
  }

  return services;
}
