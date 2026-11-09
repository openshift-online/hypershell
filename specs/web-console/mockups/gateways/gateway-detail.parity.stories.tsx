import type { Meta, StoryObj } from "@storybook/react-vite";

import { MockupParityFrame } from "../shell/mockup-template";
import { GatewayDetailPageSurface } from "./gateway-detail-page-surface";

const meta = {
  title: "Parity/Gateways/Gateway detail",
  component: GatewayDetailPageSurface,
  render: (args) => (
    <MockupParityFrame>
      <GatewayDetailPageSurface {...args} />
    </MockupParityFrame>
  ),
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof GatewayDetailPageSurface>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Connection: Story = {
  args: { activeTab: "connection" },
};

export const Provisioning: Story = {
  args: { activeTab: "connection", isProvisioning: true },
};

export const ManageAccess: Story = {
  args: { activeTab: "manage-access" },
};

export const ServiceAccounts: Story = {
  args: { activeTab: "service-accounts" },
};

export const Details: Story = {
  args: { activeTab: "details" },
};
