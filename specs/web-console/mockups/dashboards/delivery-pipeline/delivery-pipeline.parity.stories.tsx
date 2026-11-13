import type { Meta, StoryObj } from "@storybook/react-vite";

import { MockupParityFrame } from "../../shell/mockup-template";
import { DeliveryPipelinePageSurface } from "./delivery-pipeline-surface";

// Shell-free parity stories for the Glass Delivery tab
// (fleet-dashboard-delivery-pipeline.spec, FLT-DLV-16). State names match the
// design-review stories.
const meta = {
  title: "Parity/Glass/Delivery",
  component: DeliveryPipelinePageSurface,
  render: (args) => (
    <MockupParityFrame>
      <DeliveryPipelinePageSurface {...args} />
    </MockupParityFrame>
  ),
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof DeliveryPipelinePageSurface>;

export default meta;
type Story = StoryObj<typeof meta>;

export const InFlight: Story = { args: { state: "in-flight" } };
export const ChangeSelected: Story = { args: { state: "change-selected" } };
export const BuildFailed: Story = { args: { state: "build-failed" } };
export const SlowerThanUsual: Story = { args: { state: "slower-than-usual" } };
export const AllDelivered: Story = { args: { state: "all-delivered" } };
export const PromotionRetried: Story = { args: { state: "promotion-retried" } };
export const SourceUnavailable: Story = {
  args: { state: "source-unavailable" },
};
export const Loading: Story = { args: { state: "loading" } };
export const NotConfigured: Story = { args: { state: "not-configured" } };
export const Error: Story = { args: { state: "error" } };
