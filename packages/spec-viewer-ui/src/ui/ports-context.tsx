import { createContext, use, type ReactNode } from "react";

import type { SpecViewerPorts } from "../application/ports";

const PortsContext = createContext<SpecViewerPorts | null>(null);

export function PortsProvider({
  ports,
  children,
}: {
  ports: SpecViewerPorts;
  children: ReactNode;
}) {
  return <PortsContext value={ports}>{children}</PortsContext>;
}

export function usePorts(): SpecViewerPorts {
  const ports = use(PortsContext);
  if (!ports) {
    throw new Error("usePorts must be used inside PortsProvider");
  }
  return ports;
}
