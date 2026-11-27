import type { PolicyEnforcementMode } from "../common/policy-rules";
import { MockupShell } from "../shell/mockup-template";
import type {
  GatewayDetailAttention,
  GatewayDetailModal,
  GatewayDetailTab,
} from "./gateway-detail";
import { GatewayDetailPageSurface } from "./gateway-detail-page-surface";

/*
 * Full-shell design-review wrapper for the gateway detail mockup. It lives
 * apart from gateway-detail.tsx so the page-owned surface stays shell-free for
 * the Storybook parity gate (skills/build/ui-build-gate).
 */
export function GatewayDetailMockup({
  activeTab,
  attention,
  isProvisioning,
  modal,
  policyDrawer,
  policyMode,
  workspaceFilter,
}: {
  activeTab?: GatewayDetailTab;
  attention?: GatewayDetailAttention;
  isProvisioning?: boolean;
  modal?: GatewayDetailModal;
  policyDrawer?: string;
  policyMode?: PolicyEnforcementMode;
  workspaceFilter?: string;
}) {
  return (
    <MockupShell activeSection="gateways">
      <GatewayDetailPageSurface
        activeTab={activeTab}
        attention={attention}
        isProvisioning={isProvisioning}
        modal={modal}
        policyDrawer={policyDrawer}
        policyMode={policyMode}
        workspaceFilter={workspaceFilter}
      />
    </MockupShell>
  );
}
