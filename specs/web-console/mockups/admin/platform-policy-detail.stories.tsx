import type { Meta, StoryObj } from "@storybook/react-vite";

import { PlatformPolicyDetailMockup } from "./platform-policy-detail";

const meta = {
  title: "Mockups/Administration/Platform policy detail",
  component: PlatformPolicyDetailMockup,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof PlatformPolicyDetailMockup>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { args: { activeTab: "overview" } };

export const Gateways: Story = { args: { activeTab: "gateways" } };

export const AuditFindings: Story = { args: { activeTab: "findings" } };
