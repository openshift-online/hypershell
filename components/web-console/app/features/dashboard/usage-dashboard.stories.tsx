import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  createDashboardOperations,
  DashboardUiProvider,
  UsageDashboardPage,
  type DashboardControlPlane,
  type DashboardOperations,
  type DashboardUiNavigation,
} from "@openshift-online/hypershell-operational-dashboard-ui";
import { mockUsageDashboardMetrics } from "@openshift-online/hypershell-operational-dashboard-ui/fixtures";
import { IntlProvider } from "react-intl";
import { MemoryRouter, Route, Routes } from "react-router";
import { expect, within } from "storybook/test";

import { createMockDashboardControlPlane } from "../../adapters/mock/dashboard-control-plane";
import { englishMessages } from "../../i18n/catalog";
import { ApplicationShell } from "../shell/application-shell";
import { routePaths } from "../../../shared/routes";

const stubNavigation: DashboardUiNavigation = {
  adoptionHref: routePaths.dashboardAdoption,
  collectionHref: routePaths.gatewayList,
  dashboardHref: routePaths.dashboard,
  inventoryHref: routePaths.dashboardInventory,
  provisioningHref: routePaths.dashboardProvisioning,
  reliabilityHref: routePaths.dashboardReliability,
  usageHref: routePaths.dashboardUsage,
  navigate: () => undefined,
};

const unusedDashboardMetrics = {
  lastSuccessfulRefresh: new Date(),
  metrics: [],
};

const stubUsageControlPlane: DashboardControlPlane = {
  getOperationalMetrics: (context) => {
    context.signal?.throwIfAborted();
    return Promise.resolve(unusedDashboardMetrics);
  },
  getReliabilityMetrics: (context) => {
    context.signal?.throwIfAborted();
    return Promise.resolve(unusedDashboardMetrics);
  },
  getAdoptionMetrics: (context) => {
    context.signal?.throwIfAborted();
    return Promise.resolve(unusedDashboardMetrics);
  },
  getUsageMetrics: (context) => {
    context.signal?.throwIfAborted();
    return Promise.resolve({
      ...mockUsageDashboardMetrics,
      lastSuccessfulRefresh: new Date(),
    });
  },
};

const mockDashboard = createDashboardOperations({
  controlPlane: createMockDashboardControlPlane(),
});

const initialLoadFailedDashboard = createDashboardOperations({
  controlPlane: {
    ...stubUsageControlPlane,
    getUsageMetrics: (context) => {
      context.signal?.throwIfAborted();
      return Promise.reject(
        new Error("Unable to reach the usage metrics service."),
      );
    },
  },
});

const partialLoadDashboard = createDashboardOperations({
  controlPlane: {
    ...stubUsageControlPlane,
    getUsageMetrics: (context) => {
      context.signal?.throwIfAborted();
      return Promise.resolve({
        failedSources: ["usage-gateways"],
        lastSuccessfulRefresh: new Date(),
        metrics: mockUsageDashboardMetrics.metrics.filter(
          (metric) => metric.id === "sandbox-utilization",
        ),
        timeRange: 30,
      });
    },
  },
});

function UsageDashboardPreview({
  dashboard,
  isAdmin = false,
}: Readonly<{
  dashboard?: DashboardOperations;
  isAdmin?: boolean;
}>) {
  return (
    <DashboardUiProvider
      dashboard={dashboard ?? mockDashboard}
      isAdmin={isAdmin}
      navigation={stubNavigation}
    >
      <UsageDashboardPage />
    </DashboardUiProvider>
  );
}

function ShellUsageDashboardPreview() {
  return (
    <MemoryRouter initialEntries={["/dashboard/usage"]}>
      <Routes>
        <Route element={<ApplicationShell />}>
          <Route
            path="/dashboard/usage"
            element={
              <UsageDashboardPage
                metrics={{
                  ...mockUsageDashboardMetrics,
                  lastSuccessfulRefresh: new Date(),
                }}
              />
            }
          />
        </Route>
      </Routes>
    </MemoryRouter>
  );
}

const pseudoMessages = Object.fromEntries(
  Object.entries(englishMessages).map(([id, message]) => [
    id,
    `［${message.replaceAll("a", "à").replaceAll("e", "ë")}］`,
  ]),
);

const meta = {
  title: "Production/Dashboards/Usage",
  component: UsageDashboardPage,
  parameters: {
    layout: "fullscreen",
  },
  render: () => <UsageDashboardPreview isAdmin />,
} satisfies Meta<typeof UsageDashboardPage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const NonAdministrator: Story = {
  render: () => <UsageDashboardPreview />,
};

export const WithRefresh: Story = {
  render: () => <UsageDashboardPreview isAdmin />,
};

export const InitialLoadFailed: Story = {
  render: () => (
    <UsageDashboardPreview dashboard={initialLoadFailedDashboard} />
  ),
};

export const PartialLoadWarning: Story = {
  render: () => <UsageDashboardPreview dashboard={partialLoadDashboard} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);

    await canvas.findByText("Some dashboard metrics are unavailable");
    await expect(canvas.getByText("Usage dashboard")).toBeVisible();
  },
};

export const InShell: Story = {
  render: () => <ShellUsageDashboardPreview />,
};

export const PseudoLocalized: Story = {
  render: () => <UsageDashboardPreview isAdmin />,
  decorators: [
    (StoryComponent) => (
      <IntlProvider locale="en-XA" messages={pseudoMessages}>
        <StoryComponent />
      </IntlProvider>
    ),
  ],
};
