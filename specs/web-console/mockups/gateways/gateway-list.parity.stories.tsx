import type { Meta, StoryObj } from "@storybook/react-vite";

import { GatewayListPageSurface } from "./gateway-list-page-surface";
import { MockupParityFrame } from "../shell/mockup-template";

const meta = {
  title: "Parity/Gateways/Gateway list",
  component: GatewayListPageSurface,
  render: (args) => (
    <MockupParityFrame>
      <GatewayListPageSurface {...args} />
    </MockupParityFrame>
  ),
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof GatewayListPageSurface>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Loaded: Story = { args: { state: "loaded" } };
