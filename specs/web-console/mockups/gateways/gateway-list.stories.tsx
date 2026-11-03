import type { Meta, StoryObj } from "@storybook/react-vite";

import { GatewayListMockup } from "./gateway-list";

const meta = {
  title: "Mockups/Gateways/Gateway list",
  component: GatewayListMockup,
  parameters: {
    layout: "fullscreen",
  },
} satisfies Meta<typeof GatewayListMockup>;

export default meta;
type Story = StoryObj<typeof meta>;

// "Show only my gateways" off: the switch is visible and unchecked, showing the
// full authorized collection (kim's and another owner's gateways).
export const Loaded: Story = {
  args: { state: "loaded", ownershipFilterOn: false },
};

// "Show only my gateways" on: the switch is checked and the list shows only the
// signed-in user's gateways. The switch stays visible so the filter can be
// turned off (WEB-UIEX-07).
export const OwnershipFilterOn: Story = {
  args: { state: "loaded", ownershipFilterOn: true },
};

// Row selection active: one eligible gateway is selected, so the toolbar
// bulk-selection control shows the indeterminate state and the selected count,
// and the Delete bulk action is enabled. The viewer-only "shared-gateway" row
// keeps a disabled checkbox with an accessible explanation (WEB-UIEX-08).
export const SelectionActive: Story = {
  args: { state: "loaded", bulk: "selection" },
};

// Confirmation dialog shown before a bulk delete. It states how many gateways
// will be deleted, identifies them, and explains the consequences; Cancel leaves
// every gateway unchanged (WEB-UIEX-08).
export const BulkDeleteConfirm: Story = {
  args: { state: "loaded", bulk: "confirm" },
};

// Partial-failure result after a bulk delete: accepted deletions are
// distinguished from completed teardown, the failed deletion stays selected, and
// it can be retried without resubmitting the accepted deletions (WEB-UIEX-08).
export const BulkDeletePartialFailure: Story = {
  args: { state: "loaded", bulk: "partial-failure" },
};

export const Empty: Story = {
  args: { state: "empty" },
};

export const Error: Story = {
  args: { state: "error" },
};
