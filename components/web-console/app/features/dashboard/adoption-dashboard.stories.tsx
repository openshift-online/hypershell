import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  AdoptionDashboardPage,
  DashboardUiProvider,
} from "@openshift-online/hypershell-operational-dashboard-ui";
import {
  mockAdoptionDashboardMetrics,
  mockAdoptionDashboardUnavailableMetrics,
} from "@openshift-online/hypershell-operational-dashboard-ui/fixtures";

const navigation = {
  collectionHref: "/",
  navigate: () => undefined,
};

function AdoptionPreview({
  metrics,
}: Readonly<{
  metrics: typeof mockAdoptionDashboardMetrics;
}>) {
  return (
    <DashboardUiProvider
      dashboard={{
        getOperationalMetrics: () =>
          Promise.resolve({ lastSuccessfulRefresh: new Date(), metrics: [] }),
        getReliabilityMetrics: () =>
          Promise.resolve({ lastSuccessfulRefresh: new Date(), metrics: [] }),
        getAdoptionMetrics: () => Promise.resolve(metrics),
      }}
      isAdmin
      navigation={navigation}
    >
      <AdoptionDashboardPage metrics={metrics} />
    </DashboardUiProvider>
  );
}

const meta = {
  title: "Production/Dashboards/Adoption",
  component: AdoptionDashboardPage,
  parameters: { layout: "fullscreen" },
  render: () => <AdoptionPreview metrics={mockAdoptionDashboardMetrics} />,
} satisfies Meta<typeof AdoptionDashboardPage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const UnavailableMetrics: Story = {
  render: () => (
    <AdoptionPreview metrics={mockAdoptionDashboardUnavailableMetrics} />
  ),
};
