import type { Meta, StoryObj } from "@storybook/react-vite";

import { MockupParityFrame } from "../shell/mockup-template";
import { PlatformPolicyDetailPageSurface } from "./platform-policy-detail";

const meta = {
  title: "Parity/Administration/Platform policy detail",
  component: PlatformPolicyDetailPageSurface,
  render: (args) => (
    <MockupParityFrame>
      <PlatformPolicyDetailPageSurface {...args} />
    </MockupParityFrame>
  ),
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof PlatformPolicyDetailPageSurface>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { args: { activeTab: "overview" } };
