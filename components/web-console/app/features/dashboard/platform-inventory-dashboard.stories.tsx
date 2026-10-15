import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  createDashboardOperations,
  DashboardUiProvider,
  PlatformInventoryDashboardPage,
  type DashboardUiNavigation,
} from "@openshift-online/hypershell-operational-dashboard-ui";
import { IntlProvider } from "react-intl";

import { englishMessages } from "../../i18n/catalog";

const stubNavigation: DashboardUiNavigation = {
  collectionHref: "/",
  navigate: () => undefined,
};

const deterministicMetrics = {
  lastSuccessfulRefresh: new Date("2026-10-13T09:00:00.000Z"),
  metrics: [
    {
      createdLast7Days: "1",
      createdLast30Days: "2",
      id: "managed-clusters",
      inventoryProviders: {
        AWS: 4,
        "IBM Cloud": 3,
        Other: 1,
      },
      inventoryRegions: {
        "us-east (AWS)": 3,
        "us-central (IBM Cloud)": 2,
        "eu-west (IBM Cloud)": 2,
        "ap-south (Other)": 1,
      },
      inventoryStatus: {
        Failed: 1,
        Ready: 7,
      },
      value: "8",
    },
  ],
};

const deterministicDashboard = createDashboardOperations({
  controlPlane: {
    getOperationalMetrics: () => Promise.resolve(deterministicMetrics),
    getReliabilityMetrics: () =>
      Promise.resolve({ lastSuccessfulRefresh: new Date(), metrics: [] }),
  },
});

function InventoryPreview({
  isLoading = false,
  timeRange,
}: Readonly<{
  isLoading?: boolean;
  timeRange: 7 | 30;
}>) {
  return (
    <IntlProvider locale="en" messages={englishMessages}>
      <DashboardUiProvider
        dashboard={deterministicDashboard}
        isAdmin
        navigation={stubNavigation}
      >
        <PlatformInventoryDashboardPage
          isLoading={isLoading}
          isRefreshing={false}
          metrics={isLoading ? undefined : deterministicMetrics}
          onRefresh={() => undefined}
          onTimeRangeChange={() => undefined}
          timeRange={timeRange}
        />
      </DashboardUiProvider>
    </IntlProvider>
  );
}

const meta = {
  title: "Production/Dashboards/Platform inventory",
  component: PlatformInventoryDashboardPage,
  parameters: { layout: "fullscreen" },
  render: () => <InventoryPreview timeRange={30} />,
} satisfies Meta<typeof PlatformInventoryDashboardPage>;

export default meta;
type Story = StoryObj<typeof meta>;

const storyArgs = {
  isLoading: false,
  isRefreshing: false,
  onRefresh: () => undefined,
  onTimeRangeChange: () => undefined,
  timeRange: 30,
} as const;

export const Default: Story = { args: storyArgs };

export const Last7Days: Story = {
  args: { ...storyArgs, timeRange: 7 },
  render: () => <InventoryPreview timeRange={7} />,
};

export const Loading: Story = {
  args: { ...storyArgs, isLoading: true, timeRange: 7 },
  render: () => <InventoryPreview isLoading timeRange={7} />,
};
