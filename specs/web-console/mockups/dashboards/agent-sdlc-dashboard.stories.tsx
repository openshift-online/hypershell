import type { Meta, StoryObj } from "@storybook/react-vite";
import { AgentSdlcDashboardMockup } from "./agent-sdlc-dashboard";
const meta = {
  title: "Mockups/Dashboards/Agent SDLC",
  component: AgentSdlcDashboardMockup,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof AgentSdlcDashboardMockup>;
export default meta;
export const Default: StoryObj<typeof meta> = {};
