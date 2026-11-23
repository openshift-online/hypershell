import type { Meta, StoryObj } from "@storybook/react-vite";

import { MockupParityFrame } from "../shell/mockup-template";
import { PlatformPoliciesPageSurface } from "./platform-policies";

const meta = {
  title: "Parity/Administration/Platform policies",
  component: PlatformPoliciesPageSurface,
  render: (args) => (
    <MockupParityFrame>
      <PlatformPoliciesPageSurface {...args} />
    </MockupParityFrame>
  ),
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof PlatformPoliciesPageSurface>;

export default meta;
type Story = StoryObj<typeof meta>;

export const AuditOnly: Story = { args: { mode: "audit" } };

export const Enforced: Story = { args: { mode: "enforce" } };

export const Empty: Story = { args: { mode: "audit", state: "empty" } };
