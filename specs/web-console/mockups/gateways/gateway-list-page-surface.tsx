import "@patternfly/react-core/dist/styles/base.css";

import { IntlProvider } from "react-intl";

import {
  Content,
  Flex,
  FlexItem,
  PageSection,
  Title,
} from "@patternfly/react-core";

import { ResourceRefreshButton } from "../../../../packages/gateway-management-ui/src/shared/resource-refresh-button";

import {
  GatewayListContent,
  type GatewayBulkScenario,
  type GatewayListState,
} from "./gateway-list";

export function GatewayListPageSurface({
  bulk = "none",
  connectionModalGateway,
  ownershipFilterOn = false,
  state,
}: {
  bulk?: GatewayBulkScenario;
  connectionModalGateway?: string;
  ownershipFilterOn?: boolean;
  state: GatewayListState;
}) {
  return (
    <IntlProvider locale="en">
      {state === "error" ? null : (
        <PageSection
          data-page-region="header"
          hasBodyWrapper={false}
          variant="default"
        >
          <Flex
            alignItems={{ default: "alignItemsCenter" }}
            justifyContent={{ default: "justifyContentSpaceBetween" }}
          >
            <FlexItem>
              <Content>
                <Title headingLevel="h1" size="2xl">
                  OpenShell Gateways
                </Title>
              </Content>
            </FlexItem>
            <FlexItem>
              <ResourceRefreshButton
                ariaLabel="Refresh gateways"
                onRefresh={() => undefined}
              />
            </FlexItem>
          </Flex>
        </PageSection>
      )}
      <PageSection
        data-page-region="body"
        hasBodyWrapper={false}
        isFilled
        variant="secondary"
      >
        <GatewayListContent
          bulk={bulk}
          connectionModalGateway={connectionModalGateway}
          ownershipFilterOn={ownershipFilterOn}
          state={state}
        />
      </PageSection>
    </IntlProvider>
  );
}
