import type { Meta, StoryObj } from "@storybook/react-vite";

import { ReliabilityDashboardMockup } from "./reliability-dashboard";

const meta = {
  title: "Mockups/Dashboards/Reliability dashboard",
  component: ReliabilityDashboardMockup,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof ReliabilityDashboardMockup>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
