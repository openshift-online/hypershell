import type { Meta, StoryObj } from "@storybook/react-vite";
import { AdoptionDashboardPageSurface } from "./adoption-dashboard";
const meta = {
  title: "Parity/Dashboards/Adoption",
  component: AdoptionDashboardPageSurface,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof AdoptionDashboardPageSurface>;
export default meta;
export const Default: StoryObj<typeof meta> = {};
