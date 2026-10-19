import type { Meta, StoryObj } from "@storybook/react-vite";
import { CostDashboardPageSurface } from "./cost-dashboard";
const meta = {
  title: "Parity/Dashboards/Cost",
  component: CostDashboardPageSurface,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof CostDashboardPageSurface>;
export default meta;
export const Default: StoryObj<typeof meta> = {};
