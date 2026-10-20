import type { Meta, StoryObj } from "@storybook/react-vite";

import { OperationalDashboardMockup } from "./operational-dashboard";

const meta = {
  title: "Mockups/Dashboards/Operational dashboard",
  component: OperationalDashboardMockup,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof OperationalDashboardMockup>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
