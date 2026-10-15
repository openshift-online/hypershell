import type { Meta, StoryObj } from "@storybook/react-vite";
import { AdoptionDashboardMockup } from "./adoption-dashboard";
const meta = {
  title: "Mockups/Dashboards/Adoption dashboard",
  component: AdoptionDashboardMockup,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof AdoptionDashboardMockup>;
export default meta;
export const Default: StoryObj<typeof meta> = {};
