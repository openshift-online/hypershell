import {
  defaultGatewayListRequest,
  gatewayListQueryKey,
  gatewayListQueryRoot,
} from "@openshift-online/hypershell-gateway-management-ui";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { createHomepageAdapter } from "../../adapters/api/homepage";
import { createBrowserRandomSource } from "../../adapters/security/browser-random";
import {
  createQuickCreateGateway,
  createQuickCreateNameGenerator,
  type QuickCreatePlacement,
} from "../../application/homepage/quick-create";
import { gatewayOperations } from "../../composition/gateway-composition";
import { hasDashboardAdminRole } from "../../lib/session-roles";
import { useSession } from "../shell/use-session";

const homepageGateway = createHomepageAdapter();
const randomSource = createBrowserRandomSource();
const generateQuickCreateName = createQuickCreateNameGenerator(randomSource);

export const homepageSummaryQueryKey = [
  ...gatewayListQueryRoot,
  "homepage-summary",
] as const;

export const homepagePlatformOverviewQueryKey = [
  ...gatewayListQueryRoot,
  "homepage-platform-overview",
] as const;

export function useHomepageSummary() {
  return useQuery({
    queryFn: ({ signal }) => homepageGateway.getHomepageSummary(signal),
    queryKey: homepageSummaryQueryKey,
  });
}

export function useHomepagePlatformOverview() {
  const sessionQuery = useSession();
  const isAdmin = hasDashboardAdminRole(sessionQuery.data?.roles ?? []);
  return useQuery({
    enabled: sessionQuery.isSuccess && isAdmin,
    queryFn: ({ signal }) =>
      homepageGateway.getHomepagePlatformOverview(signal),
    queryKey: homepagePlatformOverviewQueryKey,
  });
}

export function useQuickCreateGateway(
  onCreated: (gatewayId: string) => void,
  enabled: boolean,
  placement: QuickCreatePlacement | undefined,
) {
  const queryClient = useQueryClient();
  const knownGateways = useQuery({
    enabled,
    queryFn: ({ signal }) =>
      gatewayOperations.listGateways(defaultGatewayListRequest, signal),
    queryKey: gatewayListQueryKey(defaultGatewayListRequest),
  });

  return useMutation({
    mutationFn: () =>
      placement === undefined
        ? Promise.reject(new Error("Quick create placement is unavailable"))
        : createQuickCreateGateway({
            generateName: generateQuickCreateName,
            knownNames: new Set(
              (knownGateways.data?.items ?? []).map((gateway) => gateway.name),
            ),
            provisionGateway: (name) =>
              gatewayOperations.provisionGateway({
                name,
                placement: { ...placement },
              }),
          }),
    onSuccess: async (gateway) => {
      await queryClient.invalidateQueries({ queryKey: gatewayListQueryRoot });
      onCreated(gateway.id);
    },
  });
}
