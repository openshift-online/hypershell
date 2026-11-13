import "@patternfly/react-core/dist/styles/base.css";

import { IntlProvider } from "react-intl";

import {
  Button,
  Content,
  Flex,
  FlexItem,
  PageSection,
  Title,
} from "@patternfly/react-core";
import { SyncAltIcon } from "@patternfly/react-icons";

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
            <Flex
              alignItems={{ default: "alignItemsCenter" }}
              gap={{ default: "gapXs" }}
            >
              <span
                style={{
                  color: "var(--pf-t--global--text--color--subtle)",
                  fontSize: "var(--pf-t--global--font--size--sm)",
                  whiteSpace: "nowrap",
                }}
              >
                Last refreshed: 9:41 AM
              </span>
              <Button
                aria-label="Refresh gateways"
                icon={<SyncAltIcon />}
                onClick={() => undefined}
                title="Refresh gateways"
                variant="plain"
              />
            </Flex>
          </FlexItem>
        </Flex>
      </PageSection>
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
