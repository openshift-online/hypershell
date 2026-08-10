import {
  createContext,
  type MouseEvent,
  type PropsWithChildren,
  useContext,
} from "react";

import type { GatewayOperations } from "./application/gateway-types";

export interface GatewayNavigationOptions {
  replace?: boolean;
  state?: unknown;
}

export interface GatewayUiNavigation {
  collectionHref: string;
  createHref: string;
  detailHref: (gatewayId: string) => string;
  navigate: (
    href: string,
    options?: GatewayNavigationOptions,
  ) => Promise<void> | void;
}

export interface GatewayUiServices {
  gateways: GatewayOperations;
  navigation: GatewayUiNavigation;
}

const GatewayUiContext = createContext<GatewayUiServices | undefined>(
  undefined,
);

export function GatewayUiProvider({
  children,
  gateways,
  navigation,
}: PropsWithChildren<GatewayUiServices>) {
  return (
    <GatewayUiContext.Provider value={{ gateways, navigation }}>
      {children}
    </GatewayUiContext.Provider>
  );
}

export function useGatewayUi(): GatewayUiServices {
  const services = useContext(GatewayUiContext);
  if (!services) {
    throw new Error("Gateway UI must be rendered within GatewayUiProvider");
  }

  return services;
}

export function useGatewayLink(href: string) {
  const { navigation } = useGatewayUi();

  return {
    href,
    onClick: (event: MouseEvent<HTMLElement>) => {
      if (
        event.defaultPrevented ||
        event.button !== 0 ||
        event.metaKey ||
        event.ctrlKey ||
        event.shiftKey ||
        event.altKey
      ) {
        return;
      }

      event.preventDefault();
      void navigation.navigate(href);
    },
  };
}
