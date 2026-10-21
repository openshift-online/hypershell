import type { Meta, StoryObj } from "@storybook/react-vite";
import { MemoryRouter } from "react-router";
import { useState } from "react";

import type {
  HomepagePlatformOverview,
  HomepageSummary,
} from "../../adapters/api/homepage";
import { HomepagePageSurface } from "./homepage";

const summary: HomepageSummary = {
  gatewayHealth: {
    attention: [
      {
        gatewayId: "research-gateway",
        name: "research-gateway",
        reason: "Connection health is degraded",
        status: "Degraded",
      },
      {
        gatewayId: "team-gateway",
        name: "team-gateway",
        reason: "Provisioning failed",
        status: "Failed",
      },
    ],
    attentionCount: 2,
    healthy: 3,
    total: 5,
  },
  supportChannel: {
    channelLabel: "#forum-hypershell",
    description:
      "Onboarding, access to a gateway, questions, or something broken. We answer in the channel.",
    title: "Talk to the team in #forum-hypershell",
    url: "https://app.slack.com/client/E030G10V24F/C0BR5DM47Q9",
  },
};

const platformOverview: HomepagePlatformOverview = {
  clusters: {
    healthy: 5,
    total: 6,
    unhealthyClusters: [
      {
        name: "production-central",
        reason: "Cluster heartbeat is stale",
        status: "Unavailable",
      },
    ],
  },
  gatewayStatuses: {
    available: true,
    counts: { Failed: 2, Provisioning: 5, Running: 35 },
    total: 42,
  },
  sandboxStatuses: {
    available: true,
    counts: { active: 112, expiring: 9, idle: 5, orphaned: 2 },
    total: 128,
  },
};

function HomepagePreview({ isAdmin = true }: Readonly<{ isAdmin?: boolean }>) {
  const [quickCreateOpen, setQuickCreateOpen] = useState(false);
  return (
    <MemoryRouter>
      <HomepagePageSurface
        isAdmin={isAdmin}
        platformOverview={platformOverview}
        platformOverviewIsLoading={false}
        quickCreate={() => {
          setQuickCreateOpen(false);
        }}
        quickCreateError={false}
        quickCreateIsPending={false}
        quickCreateOpen={quickCreateOpen}
        sessionIsLoading={false}
        setQuickCreateOpen={setQuickCreateOpen}
        summary={summary}
        summaryIsLoading={false}
      />
    </MemoryRouter>
  );
}

const meta = {
  title: "Production/Homepage",
  component: HomepagePreview,
  parameters: { layout: "fullscreen" },
  render: ({ isAdmin }) => <HomepagePreview isAdmin={isAdmin} />,
} satisfies Meta<typeof HomepagePreview>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { args: { isAdmin: true } };

export const NonAdministrator: Story = {
  args: { isAdmin: false },
};

export const PartialPlatformData: Story = {
  render: () => (
    <MemoryRouter>
      <HomepagePageSurface
        isAdmin
        platformOverview={{
          clusters: platformOverview.clusters,
          gatewayStatuses: platformOverview.gatewayStatuses,
          sandboxStatuses: { available: false },
        }}
        platformOverviewIsLoading={false}
        quickCreate={() => undefined}
        quickCreateError={false}
        quickCreateIsPending={false}
        quickCreateOpen={false}
        sessionIsLoading={false}
        setQuickCreateOpen={() => undefined}
        summary={summary}
        summaryIsLoading={false}
      />
    </MemoryRouter>
  ),
};
