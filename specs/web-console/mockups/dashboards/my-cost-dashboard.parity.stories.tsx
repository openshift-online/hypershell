import type { Meta, StoryObj } from "@storybook/react-vite";
import { MyCostDashboardPageSurface } from "./my-cost-dashboard";
const meta = {
  title: "Parity/Dashboards/My cost",
  component: MyCostDashboardPageSurface,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof MyCostDashboardPageSurface>;
export default meta;
export const Default: StoryObj<typeof meta> = {};
export const Administrator: StoryObj<typeof meta> = {
  render: () => <MyCostDashboardPageSurface isAdmin />,
};
