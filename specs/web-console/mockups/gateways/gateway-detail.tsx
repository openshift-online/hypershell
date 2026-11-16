import { MockupShell } from "../shell/mockup-template";

import {
  GatewayDetailPageSurface,
  type GatewayDetailAttention,
  type GatewayDetailModal,
  type GatewayDetailTab,
} from "./gateway-detail-page-surface";

export {
  GatewayDetailPageSurface,
  type GatewayDetailAttention,
  type GatewayDetailModal,
  type GatewayDetailTab,
};

export function GatewayDetailMockup({
  activeTab,
  attention,
  isProvisioning,
  modal,
  workspaceFilter,
}: {
  activeTab?: GatewayDetailTab;
  attention?: GatewayDetailAttention;
  isProvisioning?: boolean;
  modal?: GatewayDetailModal;
  workspaceFilter?: string;
}) {
  return (
    <MockupShell activeSection="gateways">
      <GatewayDetailPageSurface
        activeTab={activeTab}
        attention={attention}
        isProvisioning={isProvisioning}
        modal={modal}
        workspaceFilter={workspaceFilter}
      />
    </MockupShell>
  );
}
