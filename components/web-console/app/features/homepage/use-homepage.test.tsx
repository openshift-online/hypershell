import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useCallback, useState } from "react";
import { describe, expect, it, vi } from "vitest";

import {
  defaultGatewayListRequest,
  gatewayListQueryKey,
  gatewayListQueryRoot,
  type GatewayRecord,
} from "@openshift-online/hypershell-gateway-management-ui";

import {
  useHomepagePlatformOverview,
  useQuickCreateGateway,
} from "./use-homepage";

const {
  browserRandomSourceMock,
  gatewayOperationsMock,
  homepageAdapterMock,
  sessionMock,
} = vi.hoisted(() => ({
  browserRandomSourceMock: {
    nextHex: vi.fn((digits: number) => "0".repeat(digits)),
    nextInt: vi.fn(() => 0),
  },
  gatewayOperationsMock: {
    listGateways: vi.fn(),
    provisionGateway: vi.fn(),
  },
  homepageAdapterMock: {
    getHomepagePlatformOverview: vi.fn(),
    getHomepageSummary: vi.fn(),
  },
  sessionMock: vi.fn(),
}));

const createGatewayLabel = "Create gateway";

vi.mock("../../adapters/api/homepage", () => ({
  createHomepageAdapter: () => homepageAdapterMock,
}));

vi.mock("../../adapters/security/browser-random", () => ({
  createBrowserRandomSource: () => browserRandomSourceMock,
}));

vi.mock("../../composition/gateway-composition", () => ({
  gatewayOperations: gatewayOperationsMock,
}));

vi.mock("../shell/use-session", () => ({
  useSession: sessionMock,
}));

function PlatformOverviewHarness() {
  const overview = useHomepagePlatformOverview();
  return <div>{overview.fetchStatus}</div>;
}

function QuickCreateHarness() {
  const [createdGatewayId, setCreatedGatewayId] = useState<string>();
  const quickCreate = useQuickCreateGateway(setCreatedGatewayId, true);

  const create = useCallback(() => {
    void quickCreate.mutateAsync();
  }, [quickCreate]);

  return (
    <div>
      <button onClick={create} type="button">
        {createGatewayLabel}
      </button>
      <div>{createdGatewayId}</div>
    </div>
  );
}

function createQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
}

describe("homepage browser hooks", () => {
  it("does not request platform overview for non-administrators", async () => {
    sessionMock.mockReturnValue({
      data: { authenticated: true, roles: ["viewer"] },
      isSuccess: true,
    });

    render(
      <QueryClientProvider client={createQueryClient()}>
        <PlatformOverviewHarness />
      </QueryClientProvider>,
    );

    await waitFor(() => {
      expect(screen.getByText("idle")).toBeTruthy();
    });
    expect(
      homepageAdapterMock.getHomepagePlatformOverview,
    ).not.toHaveBeenCalled();
  });

  it("invalidates gateway data and reports navigation after Quick Create", async () => {
    sessionMock.mockReturnValue({
      data: { authenticated: true, roles: ["viewer"] },
      isSuccess: true,
    });
    const gateway = {
      id: "gateway-17",
      name: "amber-beacon-0000",
    } as GatewayRecord;
    const existingGateway = {
      id: "gateway-existing",
      name: "existing-gateway",
    } as GatewayRecord;
    gatewayOperationsMock.listGateways.mockResolvedValue({
      items: [existingGateway],
    });
    gatewayOperationsMock.provisionGateway.mockResolvedValue(gateway);
    const queryClient = createQueryClient();
    const invalidateQueries = vi.spyOn(queryClient, "invalidateQueries");
    const user = userEvent.setup();

    render(
      <QueryClientProvider client={queryClient}>
        <QuickCreateHarness />
      </QueryClientProvider>,
    );
    await waitFor(() => {
      expect(
        queryClient.getQueryData(
          gatewayListQueryKey(defaultGatewayListRequest),
        ),
      ).toEqual({ items: [existingGateway] });
    });

    await user.click(screen.getByRole("button", { name: "Create gateway" }));

    await waitFor(() => {
      expect(screen.getByText("gateway-17")).toBeTruthy();
    });
    expect(gatewayOperationsMock.provisionGateway).toHaveBeenCalledWith({
      name: "amber-beacon-0000",
      placement: {
        excludedSelector: ["runtime/kata"],
        selector: ["network/public", "provider/ibm"],
      },
    });
    expect(invalidateQueries).toHaveBeenCalledWith({
      queryKey: gatewayListQueryRoot,
    });
  });
});
