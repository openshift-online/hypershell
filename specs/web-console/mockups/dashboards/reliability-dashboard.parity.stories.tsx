import type { Meta, StoryObj } from "@storybook/react-vite";
import { ReliabilityDashboardPageSurface } from "./reliability-dashboard";
const meta = {
  title: "Parity/Dashboards/Reliability",
  component: ReliabilityDashboardPageSurface,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof ReliabilityDashboardPageSurface>;
export default meta;
export const Default: StoryObj<typeof meta> = {};
