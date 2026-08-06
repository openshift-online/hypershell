import { Alert, Bullseye, PageSection, Spinner } from "@patternfly/react-core";
import { FormattedMessage, useIntl } from "react-intl";

import { messages } from "../messages";

export function GatewayLoadState({ isError = false }: { isError?: boolean }) {
  const intl = useIntl();

  return (
    <PageSection hasBodyWrapper={false} isFilled variant="secondary">
      {isError ? (
        <Alert
          isInline
          title={intl.formatMessage(messages.gatewayLoadError)}
          variant="danger"
        >
          <FormattedMessage {...messages.gatewayLoadErrorBody} />
        </Alert>
      ) : (
        <Bullseye>
          <Spinner
            aria-label={intl.formatMessage(messages.loadingGateways)}
            size="xl"
          />
        </Bullseye>
      )}
    </PageSection>
  );
}
