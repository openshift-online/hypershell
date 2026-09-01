import {
  createContext,
  type MouseEvent,
  type PropsWithChildren,
  useContext,
} from "react";

import type { GatewayProfileOperations } from "./application/gateway-profile-types";

export interface GatewayProfileNavigationOptions {
  replace?: boolean;
  state?: unknown;
}

export interface GatewayProfileUiNavigation {
  collectionHref: string;
  createHref: string;
  detailHref: (gatewayProfileId: string) => string;
  navigate: (
    href: string,
    options?: GatewayProfileNavigationOptions,
  ) => Promise<void> | void;
}

export interface GatewayProfileUiServices {
  gatewayProfiles: GatewayProfileOperations;
  navigation: GatewayProfileUiNavigation;
}

const GatewayProfileUiContext = createContext<
  GatewayProfileUiServices | undefined
>(undefined);

export function GatewayProfileUiProvider({
  children,
  gatewayProfiles,
  navigation,
}: PropsWithChildren<GatewayProfileUiServices>) {
  return (
    <GatewayProfileUiContext.Provider value={{ gatewayProfiles, navigation }}>
      {children}
    </GatewayProfileUiContext.Provider>
  );
}

export function useGatewayProfileUi(): GatewayProfileUiServices {
  const services = useContext(GatewayProfileUiContext);
  if (!services) {
    throw new Error(
      "Gateway profile UI must be rendered within GatewayProfileUiProvider",
    );
  }

  return services;
}

export function useGatewayProfileLink(href: string) {
  const { navigation } = useGatewayProfileUi();

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
