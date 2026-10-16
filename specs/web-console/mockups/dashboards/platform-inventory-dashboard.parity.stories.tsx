import type { Meta, StoryObj } from "@storybook/react-vite";
import { PlatformInventoryDashboardPageSurface } from "./platform-inventory-dashboard";
const meta = {
  title: "Parity/Dashboards/Platform inventory",
  component: PlatformInventoryDashboardPageSurface,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof PlatformInventoryDashboardPageSurface>;
export default meta;
export const Default: StoryObj<typeof meta> = {};
