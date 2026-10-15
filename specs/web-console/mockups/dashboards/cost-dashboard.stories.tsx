import type { Meta, StoryObj } from "@storybook/react-vite";
import { CostDashboardMockup } from "./cost-dashboard";
const meta = {
  title: "Mockups/Dashboards/Cost dashboard",
  component: CostDashboardMockup,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof CostDashboardMockup>;
export default meta;
export const Default: StoryObj<typeof meta> = {};
