import type { Meta, StoryObj } from "@storybook/react-vite";
import { HubClusterDashboardMockup } from "./hub-cluster-dashboard-mockup";
const meta = {
  title: "Mockups/Dashboards/Provisioning",
  component: HubClusterDashboardMockup,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof HubClusterDashboardMockup>;
export default meta;
export const Default: StoryObj<typeof meta> = {};
