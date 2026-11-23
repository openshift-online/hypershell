import type { Meta, StoryObj } from "@storybook/react-vite";

import { MockupParityFrame } from "../shell/mockup-template";
import { PlatformPolicyFormPageSurface } from "./platform-policy-form";

const meta = {
  title: "Parity/Administration/Create platform policy",
  component: PlatformPolicyFormPageSurface,
  render: (args) => (
    <MockupParityFrame>
      <PlatformPolicyFormPageSurface {...args} />
    </MockupParityFrame>
  ),
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof PlatformPolicyFormPageSurface>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { args: { state: "valid" } };

export const ValidationProblems: Story = { args: { state: "invalid" } };

export const TestRequest: Story = { args: { panel: "test" } };
