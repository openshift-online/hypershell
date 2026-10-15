import type { Meta, StoryObj } from "@storybook/react-vite";

import { MockupParityFrame } from "../shell/mockup-template";
import { HubClusterDashboardPageSurface } from "./hub-cluster-dashboard";

const meta = {
  title: "Parity/Dashboards/Provisioning",
  component: HubClusterDashboardPageSurface,
  render: () => (
    <MockupParityFrame>
      <HubClusterDashboardPageSurface />
    </MockupParityFrame>
  ),
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof HubClusterDashboardPageSurface>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
