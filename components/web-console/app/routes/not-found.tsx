import {
  EmptyState,
  EmptyStateBody,
  EmptyStateVariant,
  PageSection,
} from "@patternfly/react-core";
import { FormattedMessage } from "react-intl";

import { messages } from "../i18n/messages";

export default function NotFoundRoute() {
  return (
    <PageSection hasBodyWrapper={false} isFilled>
      <EmptyState
        variant={EmptyStateVariant.full}
        titleText={<FormattedMessage {...messages.notFoundTitle} />}
        headingLevel="h1"
      >
        <EmptyStateBody>
          <FormattedMessage {...messages.notFoundBody} />
        </EmptyStateBody>
      </EmptyState>
    </PageSection>
  );
}
