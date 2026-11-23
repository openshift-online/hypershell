import type { Meta, StoryObj } from "@storybook/react-vite";

import { PlatformPoliciesMockup } from "./platform-policies";

const meta = {
  title: "Mockups/Administration/Platform policies",
  component: PlatformPoliciesMockup,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof PlatformPoliciesMockup>;

export default meta;
type Story = StoryObj<typeof meta>;

export const AuditOnly: Story = { args: { mode: "audit" } };

export const Enforced: Story = { args: { mode: "enforce" } };

export const Empty: Story = { args: { mode: "audit", state: "empty" } };

export const SwitchToEnforced: Story = {
  args: { mode: "audit", modal: "switch-to-enforced" },
};

export const SwitchToAuditOnly: Story = {
  args: { mode: "enforce", modal: "switch-to-audit" },
};

export const StartEnforcingPolicy: Story = {
  args: { mode: "audit", modal: "start-enforcing" },
};

export const DeletePolicy: Story = {
  args: { mode: "audit", modal: "delete-policy" },
};
