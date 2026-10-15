import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  DashboardUiProvider,
  ProvisioningDashboardPage,
  createDashboardOperations,
  type DashboardUiNavigation,
} from "@openshift-online/hypershell-operational-dashboard-ui";
import { mockOperationalDashboardMetrics } from "@openshift-online/hypershell-operational-dashboard-ui/fixtures";
import { IntlProvider } from "react-intl";

import { englishMessages } from "../../i18n/catalog";

const navigation: DashboardUiNavigation = {
  collectionHref: "/",
  navigate: () => undefined,
};

const dashboard = createDashboardOperations({
  controlPlane: {
    getOperationalMetrics: () =>
      Promise.resolve(mockOperationalDashboardMetrics),
    getReliabilityMetrics: () =>
      Promise.resolve(mockOperationalDashboardMetrics),
  },
});

const provisioningMetrics = {
  ...mockOperationalDashboardMetrics,
  metrics: mockOperationalDashboardMetrics.metrics.map((metric) => {
    switch (metric.id) {
      case "provisioned-gateways":
        return { ...metric, value: "42" };
      case "memory":
        return { ...metric, total: "100", unit: "%", value: "68" };
      case "cpu":
        return { ...metric, total: "100", unit: "%", value: "54" };
      case "nodes":
        return { ...metric, status: { failed: 1, healthy: 11 }, value: "12" };
      case "pods":
        return {
          ...metric,
          podPhases: {
            failed: 4,
            pending: 8,
            running: 164,
            succeeded: 8,
            unknown: 0,
          },
          total: "184",
          value: "184",
        };
      case "provision-time":
        return {
          ...metric,
          provisionDuration: {
            ...metric.provisionDuration,
            mean: "312",
            p50: "288",
            p95: "726",
          },
        };
      case "provision-reliability":
        return {
          ...metric,
          provisionOutcomes: {
            failureCount24h: "2",
            successCount24h: "52",
            successRatePercent: "96.2",
          },
          value: "96.2",
        };
      default:
        return metric;
    }
  }),
};

const meta = {
  title: "Production/Dashboards/Provisioning",
  component: ProvisioningDashboardPage,
  parameters: { layout: "fullscreen" },
  render: (args) => {
    window.localStorage.removeItem(
      "hypershell.provisioning-dashboard.layout.v1",
    );
    return <ProvisioningDashboardPage {...args} />;
  },
  decorators: [
    (Story) => (
      <IntlProvider locale="en" messages={englishMessages}>
        <DashboardUiProvider
          dashboard={dashboard}
          isAdmin
          navigation={navigation}
        >
          <Story />
        </DashboardUiProvider>
      </IntlProvider>
    ),
  ],
} satisfies Meta<typeof ProvisioningDashboardPage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    metrics: provisioningMetrics,
    onRefresh: () => undefined,
    onTimeRangeChange: () => undefined,
    timeRange: 7,
  },
};
