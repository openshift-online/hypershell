import type { Meta, StoryObj } from "@storybook/react-vite";

import { DeliveryPipelineMockup } from "./delivery-pipeline";

// Design-review stories for the Glass Delivery tab
// (fleet-dashboard-delivery-pipeline.spec, FLT-DLV-16).
const meta = {
  title: "Mockups/Glass/Delivery",
  component: DeliveryPipelineMockup,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof DeliveryPipelineMockup>;

export default meta;
type Story = StoryObj<typeof meta>;

// One change building, two changes sharing a bundle mid-promotion, and
// delivered history (FLT-DLV-02, FLT-DLV-05, FLT-DLV-08, FLT-DLV-10).
export const InFlight: Story = { args: { state: "in-flight" } };

// Details drawer for a change (FLT-DLV-11).
export const ChangeSelected: Story = { args: { state: "change-selected" } };

// A failed build makes the summary danger and names what restarts the
// pipeline (FLT-DLV-05.03, FLT-DLV-10.03).
export const BuildFailed: Story = { args: { state: "build-failed" } };

// The bundle update pull request is overdue (FLT-DLV-04.01, FLT-DLV-07).
export const SlowerThanUsual: Story = { args: { state: "slower-than-usual" } };

// Nothing in flight (FLT-DLV-10.02).
export const AllDelivered: Story = { args: { state: "all-delivered" } };

// The bundle registry cannot be read; only the Release stage is Unknown
// (FLT-DLV-13.01).
// A change whose first bundle failed the Int hub's release analysis; a later
// bundle carried it through, and the drawer shows each attempt (FLT-DLV-18).
export const PromotionRetried: Story = { args: { state: "promotion-retried" } };

export const SourceUnavailable: Story = {
  args: { state: "source-unavailable" },
};

export const Loading: Story = { args: { state: "loading" } };

// FLT-DLV-01.02.
export const NotConfigured: Story = { args: { state: "not-configured" } };

export const Error: Story = { args: { state: "error" } };
