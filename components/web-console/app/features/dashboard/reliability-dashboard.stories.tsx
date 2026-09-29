import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  createDashboardOperations,
  DashboardUiProvider,
  ReliabilityDashboardPage,
  type DashboardControlPlane,
  type DashboardOperations,
  type DashboardUiNavigation,
} from "@openshift-online/hypershell-operational-dashboard-ui";
import { mockReliabilityDashboardMetrics } from "@openshift-online/hypershell-operational-dashboard-ui/fixtures";
import { IntlProvider } from "react-intl";
import { MemoryRouter, Route, Routes } from "react-router";
import { expect, userEvent, within } from "storybook/test";

import { createMockDashboardControlPlane } from "../../adapters/mock/dashboard-control-plane";
import { englishMessages } from "../../i18n/catalog";
import { ApplicationShell } from "../shell/application-shell";

const stubNavigation: DashboardUiNavigation = {
  collectionHref: "/",
  navigate: () => undefined,
};

const stubReliabilityControlPlane: DashboardControlPlane = {
  getOperationalMetrics: (context) => {
    context.signal?.throwIfAborted();
    return Promise.resolve({
      lastSuccessfulRefresh: new Date(),
      metrics: [],
    });
  },
  getReliabilityMetrics: (context) => {
    context.signal?.throwIfAborted();
    return Promise.resolve(mockReliabilityDashboardMetrics);
  },
};

const stubDashboard = createDashboardOperations({
  controlPlane: stubReliabilityControlPlane,
});

const mockDashboard = createDashboardOperations({
  controlPlane: createMockDashboardControlPlane(),
});

const initialLoadFailedDashboard = createDashboardOperations({
  controlPlane: {
    getOperationalMetrics: (context) => {
      context.signal?.throwIfAborted();
      return Promise.resolve({
        lastSuccessfulRefresh: new Date(),
        metrics: [],
      });
    },
    getReliabilityMetrics: (context) => {
      context.signal?.throwIfAborted();
      return Promise.resolve({
        failedSources: ["api-reliability"],
        lastSuccessfulRefresh: new Date(),
        metrics: [],
      });
    },
  },
});

const loadingDashboard = createDashboardOperations({
  controlPlane: {
    getOperationalMetrics: (context) => {
      context.signal?.throwIfAborted();
      return Promise.resolve({
        lastSuccessfulRefresh: new Date(),
        metrics: [],
      });
    },
    getReliabilityMetrics: (context) => {
      context.signal?.throwIfAborted();
      return new Promise(() => {
        // Never resolves so Storybook can show the initial loading spinner.
      });
    },
  },
});

const partialLoadDashboard = createDashboardOperations({
  controlPlane: {
    getOperationalMetrics: (context) => {
      context.signal?.throwIfAborted();
      return Promise.resolve({
        lastSuccessfulRefresh: new Date(),
        metrics: [],
      });
    },
    getReliabilityMetrics: (context) => {
      context.signal?.throwIfAborted();
      return Promise.resolve({
        failedSources: ["api-reliability"],
        lastSuccessfulRefresh: new Date(),
        metrics: mockReliabilityDashboardMetrics.metrics.filter(
          (metric) => metric.id === "api-request-rate",
        ),
      });
    },
  },
});

function createRefreshFailedDashboard(): DashboardOperations {
  let callCount = 0;

  const controlPlane: DashboardControlPlane = {
    getOperationalMetrics: (context) => {
      context.signal?.throwIfAborted();
      return Promise.resolve({
        lastSuccessfulRefresh: new Date(),
        metrics: [],
      });
    },
    getReliabilityMetrics: (context) => {
      context.signal?.throwIfAborted();
      callCount += 1;

      if (callCount === 1) {
        return Promise.resolve({
          ...mockReliabilityDashboardMetrics,
          lastSuccessfulRefresh: new Date(),
        });
      }

      return Promise.resolve({
        failedSources: ["api-reliability"],
        lastSuccessfulRefresh: new Date(),
        metrics: [],
      });
    },
  };

  return createDashboardOperations({ controlPlane });
}

function DashboardPreview({
  metrics,
  dashboard,
}: Readonly<{
  metrics?: typeof mockReliabilityDashboardMetrics;
  dashboard?: DashboardOperations;
}>) {
  return (
    <DashboardUiProvider
      dashboard={
        dashboard ?? (metrics === undefined ? mockDashboard : stubDashboard)
      }
      navigation={stubNavigation}
    >
      <ReliabilityDashboardPage metrics={metrics} />
    </DashboardUiProvider>
  );
}

function ShellDashboardPreview() {
  return (
    <MemoryRouter initialEntries={["/dashboard/reliability"]}>
      <Routes>
        <Route element={<ApplicationShell />}>
          <Route
            path="/dashboard/reliability"
            element={
              <ReliabilityDashboardPage
                metrics={mockReliabilityDashboardMetrics}
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
  title: "HyperShell/Reliability dashboard",
  component: ReliabilityDashboardPage,
  parameters: {
    layout: "fullscreen",
  },
  render: () => <DashboardPreview metrics={mockReliabilityDashboardMetrics} />,
} satisfies Meta<typeof ReliabilityDashboardPage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const MockedMetrics: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);

    await canvas.findByText("Reliability summary");

    for (const title of ["API request rate", "API error rate", "API latency"]) {
      const widgetTitle = canvas
        .getAllByText(title)
        .find((node) =>
          node.classList.contains("pf-v6-widget-grid-tile__title"),
        );
      await expect(widgetTitle).toBeDefined();
      await expect(widgetTitle).toBeVisible();
    }

    const sparklineCaptions = canvas.getAllByText("Last 24 hours");
    await expect(sparklineCaptions.length).toBeGreaterThanOrEqual(3);
    await expect(
      canvas.getByRole("button", {
        name: /\d+% increase in Request rate/u,
      }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", {
        name: /\d+% increase in Error rate/u,
      }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", {
        name: /\d+% increase in Median latency/u,
      }),
    ).toBeVisible();
  },
};

export const WithRefresh: Story = {
  render: () => <DashboardPreview />,
};

export const Loading: Story = {
  render: () => <DashboardPreview dashboard={loadingDashboard} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);

    await expect(
      canvas.getByLabelText("Loading reliability dashboard metrics"),
    ).toBeVisible();
  },
};

export const InitialLoadFailed: Story = {
  render: () => <DashboardPreview dashboard={initialLoadFailedDashboard} />,
};

export const PartialLoadWarning: Story = {
  render: () => <DashboardPreview dashboard={partialLoadDashboard} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);

    await canvas.findByText("Some dashboard metrics are unavailable");
    await expect(canvas.getByText("Reliability summary")).toBeVisible();
  },
};

export const RefreshFailed: Story = {
  render: () => <DashboardPreview dashboard={createRefreshFailedDashboard()} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);

    await canvas.findByText("Reliability summary");
    await userEvent.click(
      canvas.getByRole("button", { name: "Refresh dashboard metrics" }),
    );
    await expect(
      canvas.getByText("Some dashboard metrics are unavailable"),
    ).toBeVisible();
  },
};

export const InShell: Story = {
  render: () => <ShellDashboardPreview />,
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
