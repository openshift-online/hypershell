import type { Meta, StoryObj } from "@storybook/react-vite";

import { PlatformPolicyFormMockup } from "./platform-policy-form";

const meta = {
  title: "Mockups/Administration/Create platform policy",
  component: PlatformPolicyFormMockup,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof PlatformPolicyFormMockup>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { args: { state: "valid" } };

export const ValidationProblems: Story = { args: { state: "invalid" } };

export const Snippets: Story = { args: { panel: "snippets" } };

export const AddSnippetRule: Story = {
  args: { panel: "snippets", modal: "insert-snippet" },
};

export const TestRequest: Story = { args: { panel: "test" } };
