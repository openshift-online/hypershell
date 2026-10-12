import "@patternfly/react-core/dist/styles/base.css";

import { Content, PageSection, Title } from "@patternfly/react-core";

import { ProvisionGatewayForm } from "./provision-gateway";

export function ProvisionGatewayPageSurface({
  showValidationErrors = false,
}: {
  showValidationErrors?: boolean;
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
            Provision gateway
          </Title>
          <p>Configure a new OpenShell gateway.</p>
        </Content>
      </PageSection>
      <PageSection
        data-page-region="body"
        hasBodyWrapper={false}
        isFilled
        variant="secondary"
      >
        <ProvisionGatewayForm showValidationErrors={showValidationErrors} />
      </PageSection>
    </>
  );
}
