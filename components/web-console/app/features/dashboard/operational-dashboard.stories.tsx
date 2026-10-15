import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  createDashboardOperations,
  DashboardUiProvider,
  OperationalDashboardPage,
  type DashboardOperations,
  type DashboardUiNavigation,
} from "@openshift-online/hypershell-operational-dashboard-ui";
import { mockOperationalDashboardMetrics } from "@openshift-online/hypershell-operational-dashboard-ui/fixtures";
import { IntlProvider } from "react-intl";
import { MemoryRouter, Route, Routes } from "react-router";
import { expect, within } from "storybook/test";

import { createMockDashboardControlPlane } from "../../adapters/mock/dashboard-control-plane";
import { englishMessages } from "../../i18n/catalog";
import { ApplicationShell } from "../shell/application-shell";

const stubNavigation: DashboardUiNavigation = {
  collectionHref: "/",
  navigate: () => undefined,
};

const stubReliabilityMetrics = {
  lastSuccessfulRefresh: new Date(),
  metrics: [],
};

const stubDashboard = createDashboardOperations({
  controlPlane: {
    getOperationalMetrics: (context) => {
      context.signal?.throwIfAborted();
      return Promise.resolve(mockOperationalDashboardMetrics);
    },
    getReliabilityMetrics: (context) => {
      context.signal?.throwIfAborted();
      return Promise.resolve(stubReliabilityMetrics);
    },
  },
});

const mockDashboard = createDashboardOperations({
  controlPlane: createMockDashboardControlPlane(),
});

const initialLoadFailedDashboard = createDashboardOperations({
  controlPlane: {
    getOperationalMetrics: (context) => {
      context.signal?.throwIfAborted();
      return Promise.reject(
        new Error("Unable to reach the operational metrics service."),
      );
    },
    getReliabilityMetrics: (context) => {
      context.signal?.throwIfAborted();
      return Promise.resolve(stubReliabilityMetrics);
    },
  },
});

const partialLoadDashboard = createDashboardOperations({
  controlPlane: {
    getOperationalMetrics: (context) => {
      context.signal?.throwIfAborted();
      return Promise.resolve({
        failedSources: [
          "cluster-memory",
          "cluster-cpu",
          "cluster-pods",
          "cluster-nodes",
        ],
        lastSuccessfulRefresh: new Date(),
        metrics: mockOperationalDashboardMetrics.metrics.filter((metric) =>
          [
            "provisioned-gateways",
            "provisioned-sandboxes",
            "registered-users",
            "provision-time",
          ].includes(metric.id),
        ),
      });
    },
    getReliabilityMetrics: (context) => {
      context.signal?.throwIfAborted();
      return Promise.resolve(stubReliabilityMetrics);
    },
  },
});

function ShellDashboardPreview() {
  return (
    <MemoryRouter initialEntries={["/"]}>
      <Routes>
        <Route element={<ApplicationShell />}>
          <Route
            path="/"
            element={
              <OperationalDashboardPage
                metrics={{
                  ...mockOperationalDashboardMetrics,
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

function DashboardPreview({
  metrics,
  dashboard,
  isAdmin = false,
}: Readonly<{
  metrics?: typeof mockOperationalDashboardMetrics;
  dashboard?: DashboardOperations;
  isAdmin?: boolean;
}>) {
  return (
    <DashboardUiProvider
      dashboard={
        dashboard ?? (metrics === undefined ? mockDashboard : stubDashboard)
      }
      isAdmin={isAdmin}
      navigation={stubNavigation}
    >
      <OperationalDashboardPage
        metrics={
          metrics
            ? { ...metrics, lastSuccessfulRefresh: new Date() }
            : undefined
        }
      />
    </DashboardUiProvider>
  );
}

const pseudoMessages = Object.fromEntries(
  Object.entries(englishMessages).map(([id, message]) => [
    id,
    `［${message.replaceAll("a", "à").replaceAll("e", "ë")}］`,
  ]),
);

const meta = {
  title: "Production/Dashboards/Operational",
  component: OperationalDashboardPage,
  parameters: {
    layout: "fullscreen",
  },
  render: () => <DashboardPreview metrics={mockOperationalDashboardMetrics} />,
} satisfies Meta<typeof OperationalDashboardPage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const Administrator: Story = {
  render: () => (
    <DashboardPreview isAdmin metrics={mockOperationalDashboardMetrics} />
  ),
};

export const WithRefresh: Story = {
  render: () => <DashboardPreview />,
};

export const InitialLoadFailed: Story = {
  render: () => <DashboardPreview dashboard={initialLoadFailedDashboard} />,
};

export const InShell: Story = {
  render: () => <ShellDashboardPreview />,
};

export const PartialLoadWarning: Story = {
  render: () => <DashboardPreview dashboard={partialLoadDashboard} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);

    await canvas.findByText("Some dashboard metrics are unavailable");
    await expect(canvas.getByText("Usage summary")).toBeVisible();
  },
};

export const PseudoLocalized: Story = {
  decorators: [
    (StoryComponent) => (
      <IntlProvider locale="en-XA" messages={pseudoMessages}>
        <StoryComponent />
      </IntlProvider>
    ),
  ],
};

export const RightToLeft: Story = {
  decorators: [
    (StoryComponent) => (
      <div dir="rtl" lang="ar">
        <IntlProvider locale="ar" messages={englishMessages}>
          <StoryComponent />
        </IntlProvider>
      </div>
    ),
  ],
};
