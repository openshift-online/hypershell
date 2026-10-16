import type { Meta, StoryObj } from "@storybook/react-vite";

import { MockupParityFrame } from "../shell/mockup-template";
import { AgentSdlcDashboardPageSurface } from "./agent-sdlc-dashboard";

const meta = {
  title: "Parity/Dashboards/Agent SDLC",
  component: AgentSdlcDashboardPageSurface,
  render: () => (
    <MockupParityFrame>
      <AgentSdlcDashboardPageSurface />
    </MockupParityFrame>
  ),
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof AgentSdlcDashboardPageSurface>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
