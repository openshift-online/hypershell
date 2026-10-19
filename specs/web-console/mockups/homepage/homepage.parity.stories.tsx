import type { Meta, StoryObj } from "@storybook/react-vite";
import { HomepagePageSurface } from "./homepage-page-surface";

const meta = {
  title: "Parity/Homepage",
  component: HomepagePageSurface,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof HomepagePageSurface>;

export default meta;
type Story = StoryObj<typeof meta>;
export const Loaded: Story = { args: { state: "loaded" } };
