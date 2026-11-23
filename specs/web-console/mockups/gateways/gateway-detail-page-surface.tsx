import "@patternfly/react-core/dist/styles/base.css";

import { IntlProvider } from "react-intl";

import { GatewayDetailContent } from "./gateway-detail";

type GatewayDetailContentProps = Parameters<typeof GatewayDetailContent>[0];

export function GatewayDetailPageSurface({
  activeTab,
  attention,
  isProvisioning,
  modal,
  policyDrawer,
  policyMode,
}: Pick<
  GatewayDetailContentProps,
  | "activeTab"
  | "attention"
  | "isProvisioning"
  | "modal"
  | "policyDrawer"
  | "policyMode"
>) {
  return (
    <IntlProvider locale="en">
      <GatewayDetailContent
        activeTab={activeTab}
        attention={attention}
        isProvisioning={isProvisioning}
        modal={modal}
        policyDrawer={policyDrawer}
        policyMode={policyMode}
      />
    </IntlProvider>
  );
}
