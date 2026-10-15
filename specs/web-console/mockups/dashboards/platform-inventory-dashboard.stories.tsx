import type { Meta, StoryObj } from "@storybook/react-vite";
import { PlatformInventoryDashboardMockup } from "./platform-inventory-dashboard";
const meta = {
  title: "Mockups/Dashboards/Platform inventory",
  component: PlatformInventoryDashboardMockup,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof PlatformInventoryDashboardMockup>;
export default meta;
export const Default: StoryObj<typeof meta> = {};
