import {
  Content,
  Label,
  Page,
  PageSection,
  SkipToContent,
  Title,
} from "@patternfly/react-core";
import { FormattedMessage } from "react-intl";

import { messages } from "../../i18n/messages";

export function HelloWorld() {
  const skipToContent = (
    <SkipToContent href="#main-content">
      <FormattedMessage {...messages.skipToContent} />
    </SkipToContent>
  );

  return (
    <Page skipToContent={skipToContent} mainContainerId="main-content">
      <PageSection hasBodyWrapper={false} isFilled>
        <div>
          <Label color="red">
            <FormattedMessage {...messages.productName} />
          </Label>
          <Title headingLevel="h1" size="4xl">
            <FormattedMessage {...messages.helloTitle} />
          </Title>
          <Content>
            <p>
              <FormattedMessage {...messages.helloDescription} />
            </p>
          </Content>
        </div>
      </PageSection>
    </Page>
  );
}
