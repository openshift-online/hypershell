import type { Meta, StoryObj } from "@storybook/react-vite";

import { TemplatePreview } from "./mockup-template";

const meta = {
  title: "Mockups/Template",
  component: TemplatePreview,
  parameters: {
    layout: "fullscreen",
  },
} satisfies Meta<typeof TemplatePreview>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
