import "@patternfly/react-core/dist/styles/base.css";

import { Content, PageSection, Title } from "@patternfly/react-core";

import { GatewayListContent, type GatewayListState } from "./gateway-list";

export function GatewayListPageSurface({ state }: { state: GatewayListState }) {
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
        <GatewayListContent state={state} />
      </PageSection>
    </>
  );
}
