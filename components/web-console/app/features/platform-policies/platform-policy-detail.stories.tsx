import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";

import {
  baselineFindings,
  baselinePolicy,
  platformPolicySettings,
} from "./fixtures/platform-policy-fixtures";
import {
  PlatformPolicyDetailPage,
  type PlatformPolicyDetailTab,
  type PlatformPolicyFindingsFilter,
} from "./platform-policy-detail-page";
import type { PlatformPolicyDialog } from "./platform-policy-summary";

const links = {
  editHref: (policyId: string) => `/admin/platform-policies/${policyId}/edit`,
  gatewayHref: (gatewayId: string) => `/gateways/${gatewayId}`,
  listHref: "/admin/platform-policies",
  onNavigate: () => undefined,
};

function PlatformPolicyDetailProductionPreview({
  activeTab: initialTab,
}: {
  activeTab: PlatformPolicyDetailTab;
}) {
  const [activeTab, setActiveTab] = useState(initialTab);
  const [dialog, setDialog] = useState<PlatformPolicyDialog>();
  const [filter, setFilter] = useState<PlatformPolicyFindingsFilter>({});
  const items = baselineFindings.filter(
    (finding) =>
      (!filter.gatewayId || finding.gatewayId === filter.gatewayId) &&
      (!filter.outcome || finding.outcome === filter.outcome),
  );
  return (
    <PlatformPolicyDetailPage
      activeTab={activeTab}
      dialog={dialog}
      findings={{
        isError: false,
        isLoading: false,
        items,
        page: 1,
        size: 20,
        total: filter.gatewayId || filter.outcome ? items.length : 12,
      }}
      findingsFilter={filter}
      isError={false}
      isLoading={false}
      isNotFound={false}
      isWriting={false}
      links={links}
      onConfirmDialog={() => {
        setDialog(undefined);
      }}
      onDialogChange={setDialog}
      onDismissWriteFailure={() => undefined}
      onFindingsFilterChange={setFilter}
      onFindingsPageChange={() => undefined}
      onRetry={() => undefined}
      onTabChange={setActiveTab}
      policy={baselinePolicy()}
      settings={platformPolicySettings()}
    />
  );
}

const meta = {
  title: "Production/Administration/Platform policy detail",
  component: PlatformPolicyDetailProductionPreview,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof PlatformPolicyDetailProductionPreview>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { args: { activeTab: "overview" } };

export const Gateways: Story = { args: { activeTab: "gateways" } };

export const AuditFindings: Story = { args: { activeTab: "findings" } };
