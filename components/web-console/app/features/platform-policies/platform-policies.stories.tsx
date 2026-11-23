import type { PolicyMode } from "@openshift-online/hypershell-gateway-management-ui";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";

import {
  platformPolicies,
  platformPolicySettings,
} from "./fixtures/platform-policy-fixtures";
import { PlatformPoliciesPage } from "./platform-policies-page";
import type {
  PlatformPolicyDialog,
  PolicySortDirection,
} from "./platform-policy-summary";

const links = {
  createHref: "/admin/platform-policies/new",
  detailHref: (policyId: string) => `/admin/platform-policies/${policyId}`,
  editHref: (policyId: string) => `/admin/platform-policies/${policyId}/edit`,
  gatewaysHref: (policyId: string) =>
    `/admin/platform-policies/${policyId}?tab=gateways`,
  onNavigate: () => undefined,
};

function PlatformPoliciesProductionPreview({
  initialDialog,
  isEmpty = false,
  mode,
}: {
  initialDialog?: PlatformPolicyDialog;
  isEmpty?: boolean;
  mode: PolicyMode;
}) {
  const [dialog, setDialog] = useState(initialDialog);
  const [filterText, setFilterText] = useState("");
  const [sortDirection, setSortDirection] =
    useState<PolicySortDirection>("asc");
  return (
    <PlatformPoliciesPage
      dialog={dialog}
      filterText={filterText}
      isError={false}
      isLoading={false}
      isWriting={false}
      links={links}
      onConfirmDialog={() => {
        setDialog(undefined);
      }}
      onDialogChange={setDialog}
      onDismissWriteFailure={() => undefined}
      onFilterTextChange={setFilterText}
      onRetry={() => undefined}
      onSortDirectionChange={setSortDirection}
      policies={isEmpty ? [] : platformPolicies(mode)}
      settings={platformPolicySettings(mode)}
      sortDirection={sortDirection}
    />
  );
}

const meta = {
  title: "Production/Administration/Platform policies",
  component: PlatformPoliciesProductionPreview,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof PlatformPoliciesProductionPreview>;

export default meta;
type Story = StoryObj<typeof meta>;

export const AuditOnly: Story = { args: { mode: "audit" } };

export const Enforced: Story = { args: { mode: "enforce" } };

export const Empty: Story = { args: { isEmpty: true, mode: "audit" } };

export const SwitchToEnforced: Story = {
  args: {
    initialDialog: { kind: "global-mode", to: "enforce" },
    mode: "audit",
  },
};

export const SwitchToAuditOnly: Story = {
  args: {
    initialDialog: { kind: "global-mode", to: "audit" },
    mode: "enforce",
  },
};

export const StartEnforcingPolicy: Story = {
  args: {
    initialDialog: {
      kind: "policy-mode",
      policyId: "policy-baseline",
      to: "enforce",
    },
    mode: "audit",
  },
};

export const DeletePolicy: Story = {
  args: {
    initialDialog: { kind: "delete", policyId: "policy-baseline" },
    mode: "audit",
  },
};
