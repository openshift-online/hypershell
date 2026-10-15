import type { Meta, StoryObj } from "@storybook/react-vite";
import { UsageDashboardPageSurface } from "./usage-dashboard";
const meta = {
  title: "Parity/Dashboards/Usage",
  component: UsageDashboardPageSurface,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof UsageDashboardPageSurface>;
export default meta;
export const Default: StoryObj<typeof meta> = {};
