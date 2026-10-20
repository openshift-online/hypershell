import type { Meta, StoryObj } from "@storybook/react-vite";
import { OperationalDashboardPageSurface } from "./operational-dashboard";
const meta = {
  title: "Parity/Dashboards/Operational",
  component: OperationalDashboardPageSurface,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof OperationalDashboardPageSurface>;
export default meta;
export const Default: StoryObj<typeof meta> = {};
