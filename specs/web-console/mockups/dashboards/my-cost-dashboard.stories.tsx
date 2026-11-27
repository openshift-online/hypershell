import type { Meta, StoryObj } from "@storybook/react-vite";
import { MyCostDashboardMockup } from "./my-cost-dashboard-mockup";
const meta = {
  title: "Mockups/Dashboards/My cost dashboard",
  component: MyCostDashboardMockup,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof MyCostDashboardMockup>;
export default meta;
export const Default: StoryObj<typeof meta> = {};
