import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  CostDashboardPage,
  createDashboardOperations,
  DashboardUiProvider,
  type CostDashboardMetrics,
  type DashboardControlPlane,
  type DashboardUiNavigation,
} from "@openshift-online/hypershell-operational-dashboard-ui";
import {
  mockCostDashboardMetrics,
  mockCostDashboardNotConfiguredMetrics,
  mockMyCostDashboardMetrics,
} from "@openshift-online/hypershell-operational-dashboard-ui/fixtures";
import { expect, within } from "storybook/test";

import { routePaths } from "../../../shared/routes";

const stubNavigation: DashboardUiNavigation = {
  adoptionHref: routePaths.dashboardAdoption,
  collectionHref: routePaths.gatewayList,
  costHref: routePaths.dashboardCost,
  dashboardHref: routePaths.dashboard,
  inventoryHref: routePaths.dashboardInventory,
  provisioningHref: routePaths.dashboardProvisioning,
  reliabilityHref: routePaths.dashboardReliability,
  usageHref: routePaths.dashboardUsage,
  navigate: () => undefined,
};

const unused = { lastSuccessfulRefresh: new Date(), metrics: [] };

function costControlPlane(
  load: (view: "platform" | "mine") => CostDashboardMetrics,
): DashboardControlPlane {
  return {
    getAdoptionMetrics: () => Promise.resolve(unused),
    getCostMetrics: (context) => {
      context.signal?.throwIfAborted();
      return Promise.resolve({
        ...load(context.costView ?? "mine"),
        lastSuccessfulRefresh: new Date(),
      });
    },
    getOperationalMetrics: () => Promise.resolve(unused),
    getReliabilityMetrics: () => Promise.resolve(unused),
  };
}

const fixtures = costControlPlane((view) =>
  view === "platform" ? mockCostDashboardMetrics : mockMyCostDashboardMetrics,
);

function CostDashboardPreview({
  controlPlane = fixtures,
  isAdmin = false,
}: Readonly<{ controlPlane?: DashboardControlPlane; isAdmin?: boolean }>) {
  return (
    <DashboardUiProvider
      dashboard={createDashboardOperations({ controlPlane })}
      isAdmin={isAdmin}
      navigation={stubNavigation}
    >
      <CostDashboardPage />
    </DashboardUiProvider>
  );
}

const meta = {
  title: "Production/Dashboards/Cost",
  component: CostDashboardPage,
  parameters: { layout: "fullscreen" },
  render: () => <CostDashboardPreview isAdmin />,
} satisfies Meta<typeof CostDashboardPage>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Platform view for an administrator (matches Parity/Dashboards/Cost). */
export const Default: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(await canvas.findByText("$18,420 / month")).toBeVisible();
  },
};

/** My cost for a non-administrator (matches Parity/Dashboards/My cost). */
export const MyCost: Story = {
  render: () => <CostDashboardPreview />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(await canvas.findByText("$740 / month")).toBeVisible();
  },
};

export const NotConfigured: Story = {
  render: () => (
    <CostDashboardPreview
      controlPlane={costControlPlane(
        () => mockCostDashboardNotConfiguredMetrics,
      )}
      isAdmin
    />
  ),
};

export const NoGateways: Story = {
  render: () => (
    <CostDashboardPreview
      controlPlane={costControlPlane(() => ({
        ...mockMyCostDashboardMetrics,
        empty: true,
        mine: undefined,
      }))}
    />
  ),
};

export const PartialLoad: Story = {
  render: () => (
    <CostDashboardPreview
      controlPlane={costControlPlane(() => ({
        ...mockCostDashboardMetrics,
        failedSources: ["cost-gateways"],
        platform: {
          breakdown: mockCostDashboardMetrics.platform?.breakdown,
          summary: mockCostDashboardMetrics.platform?.summary,
        },
      }))}
      isAdmin
    />
  ),
};
