import "@patternfly/react-core/dist/styles/base.css";

import { Content, PageSection, Title } from "@patternfly/react-core";

import {
  GatewayListContent,
  type GatewayBulkScenario,
  type GatewayListState,
} from "./gateway-list";

export function GatewayListPageSurface({
  bulk = "none",
  ownershipFilterOn = false,
  state,
}: {
  bulk?: GatewayBulkScenario;
  ownershipFilterOn?: boolean;
  state: GatewayListState;
}) {
  return (
    <>
      <PageSection
        data-page-region="header"
        hasBodyWrapper={false}
        variant="default"
      >
        <Content>
          <Title headingLevel="h1" size="2xl">
            OpenShell Gateways
          </Title>
        </Content>
      </PageSection>
      <PageSection
        data-page-region="body"
        hasBodyWrapper={false}
        isFilled
        variant="secondary"
      >
        <GatewayListContent
          bulk={bulk}
          ownershipFilterOn={ownershipFilterOn}
          state={state}
        />
      </PageSection>
    </>
  );
}
