import type { Meta, StoryObj } from "@storybook/react-vite";
import { MockupShell } from "../shell/mockup-template";
import { AdoptionDashboardPageSurface } from "./adoption-dashboard";

function AdoptionDashboardMockup() {
  return (
    <MockupShell>
      <AdoptionDashboardPageSurface />
    </MockupShell>
  );
}

const meta = {
  title: "Mockups/Dashboards/Adoption dashboard",
  component: AdoptionDashboardMockup,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof AdoptionDashboardMockup>;
export default meta;
export const Default: StoryObj<typeof meta> = {};
