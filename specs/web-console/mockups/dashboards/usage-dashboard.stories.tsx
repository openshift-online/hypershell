import type { Meta, StoryObj } from "@storybook/react-vite";
import { UsageDashboardMockup } from "./usage-dashboard";
const meta = {
  title: "Mockups/Dashboards/Usage dashboard",
  component: UsageDashboardMockup,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof UsageDashboardMockup>;
export default meta;
export const Administrator: StoryObj<typeof meta> = {};
export const NonAdministrator: StoryObj<typeof meta> = {
  args: { isAdmin: false },
};
