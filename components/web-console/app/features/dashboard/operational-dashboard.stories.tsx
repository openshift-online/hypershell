import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  createDashboardOperations,
  DashboardUiProvider,
  OperationalDashboardPage,
  type DashboardControlPlane,
  type DashboardOperations,
  type DashboardUiNavigation,
} from "@openshift-online/hypershell-operational-dashboard-ui";
import { mockOperationalDashboardMetrics } from "@openshift-online/hypershell-operational-dashboard-ui/fixtures";
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

const stubDashboard = createDashboardOperations({
  controlPlane: {
    getOperationalMetrics: (context) => {
      context.signal?.throwIfAborted();
      return Promise.resolve(mockOperationalDashboardMetrics);
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
  },
});

function createRefreshFailedDashboard(): DashboardOperations {
  let callCount = 0;

  const controlPlane: DashboardControlPlane = {
    getOperationalMetrics: (context) => {
      context.signal?.throwIfAborted();
      callCount += 1;

      if (callCount === 1) {
        return Promise.resolve({
          ...mockOperationalDashboardMetrics,
          lastSuccessfulRefresh: new Date(),
        });
      }

      return Promise.reject(
        new Error("Unable to refresh operational dashboard metrics."),
      );
    },
  };

  return createDashboardOperations({ controlPlane });
}

function DashboardPreview({
  metrics,
  dashboard,
}: Readonly<{
  metrics?: typeof mockOperationalDashboardMetrics;
  dashboard?: DashboardOperations;
}>) {
  return (
    <DashboardUiProvider
      dashboard={
        dashboard ?? (metrics === undefined ? mockDashboard : stubDashboard)
      }
      navigation={stubNavigation}
    >
      <OperationalDashboardPage metrics={metrics} />
    </DashboardUiProvider>
  );
}

function ShellDashboardPreview() {
  return (
    <MemoryRouter initialEntries={["/"]}>
      <Routes>
        <Route element={<ApplicationShell />}>
          <Route
            path="/"
            element={
              <OperationalDashboardPage
                metrics={mockOperationalDashboardMetrics}
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
  title: "HyperShell/Operational dashboard",
  component: OperationalDashboardPage,
  parameters: {
    layout: "fullscreen",
  },
  render: () => <DashboardPreview metrics={mockOperationalDashboardMetrics} />,
} satisfies Meta<typeof OperationalDashboardPage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const MockedMetrics: Story = {};

export const WithRefresh: Story = {
  render: () => <DashboardPreview />,
};

export const InitialLoadFailed: Story = {
  render: () => <DashboardPreview dashboard={initialLoadFailedDashboard} />,
};

export const RefreshFailed: Story = {
  render: () => <DashboardPreview dashboard={createRefreshFailedDashboard()} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);

    await canvas.findByText("Usage summary");
    await userEvent.click(
      canvas.getByRole("button", { name: "Refresh dashboard metrics" }),
    );
    await expect(
      canvas.getByText("Could not refresh dashboard metrics"),
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
