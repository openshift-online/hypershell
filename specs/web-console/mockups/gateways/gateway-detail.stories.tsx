import type { Meta, StoryObj } from "@storybook/react-vite";

import { GatewayDetailMockup } from "./gateway-detail";

const meta = {
  title: "Mockups/Gateways/Gateway detail",
  component: GatewayDetailMockup,
  parameters: {
    layout: "fullscreen",
  },
} satisfies Meta<typeof GatewayDetailMockup>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Connection: Story = {
  args: { activeTab: "connection" },
};

export const Provisioning: Story = {
  args: { activeTab: "connection", isProvisioning: true },
};

export const UserAccessWhileProvisioning: Story = {
  args: { activeTab: "manage-access", isProvisioning: true },
};

export const ServiceAccountsWhileProvisioning: Story = {
  args: { activeTab: "service-accounts", isProvisioning: true },
};

export const UserAccess: Story = {
  args: { activeTab: "manage-access" },
};

export const ServiceAccounts: Story = {
  args: { activeTab: "service-accounts" },
};

export const Details: Story = {
  args: { activeTab: "details" },
};

export const GatewayHealthAttention: Story = {
  args: { activeTab: "details", attention: "gateway-health" },
};

export const SandboxExpiringAttention: Story = {
  args: { activeTab: "details", attention: "sandbox-expiring" },
};

export const SandboxIdleAttention: Story = {
  args: { activeTab: "details", attention: "sandbox-idle" },
};

export const ServiceAccountExpiringAttention: Story = {
  args: {
    activeTab: "details",
    attention: "service-account-expiring",
  },
};

export const RenameGateway: Story = {
  args: { activeTab: "connection", modal: "rename-gateway" },
};

export const DeleteGateway: Story = {
  args: { activeTab: "connection", modal: "delete-gateway" },
};

export const AddUsers: Story = {
  args: { activeTab: "manage-access", modal: "add-users" },
};

export const WorkspaceAccess: Story = {
  args: { activeTab: "manage-access", modal: "workspace-access" },
};

export const ChangeOwnRole: Story = {
  args: { activeTab: "manage-access", modal: "change-own-role" },
};

export const RemoveAccess: Story = {
  args: { activeTab: "manage-access", modal: "remove-access" },
};

export const CreateServiceAccount: Story = {
  args: { activeTab: "service-accounts", modal: "create-service-account" },
};

export const ServiceAccountSetup: Story = {
  args: { activeTab: "service-accounts", modal: "service-account-setup" },
};

export const EditServiceAccount: Story = {
  args: { activeTab: "service-accounts", modal: "edit-service-account" },
};

export const RevokeServiceAccount: Story = {
  args: { activeTab: "service-accounts", modal: "revoke-service-account" },
};

export const DeleteServiceAccount: Story = {
  args: { activeTab: "service-accounts", modal: "delete-service-account" },
};
