import {
  EmptyState,
  EmptyStateBody,
  EmptyStateVariant,
} from "@patternfly/react-core";
import { FormattedMessage } from "react-intl";

import { messages } from "../i18n/messages";

export default function NotFoundRoute() {
  return (
    <main id="main-content">
      <EmptyState
        variant={EmptyStateVariant.full}
        titleText={<FormattedMessage {...messages.notFoundTitle} />}
        headingLevel="h1"
      >
        <EmptyStateBody>
          <FormattedMessage {...messages.notFoundBody} />
        </EmptyStateBody>
      </EmptyState>
    </main>
  );
}
