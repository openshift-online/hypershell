import { Alert, Bullseye, PageSection, Spinner } from "@patternfly/react-core";
import { FormattedMessage, useIntl } from "react-intl";

import { gatewayProfileMessages } from "../gateway-profile-messages";

export function GatewayProfileLoadState({
  isError = false,
}: {
  isError?: boolean;
}) {
  const intl = useIntl();

  return (
    <PageSection hasBodyWrapper={false} isFilled variant="secondary">
      {isError ? (
        <Alert
          isInline
          title={intl.formatMessage(
            gatewayProfileMessages.gatewayProfileLoadError,
          )}
          variant="danger"
        >
          <FormattedMessage
            {...gatewayProfileMessages.gatewayProfileLoadErrorBody}
          />
        </Alert>
      ) : (
        <Bullseye>
          <Spinner
            aria-label={intl.formatMessage(
              gatewayProfileMessages.loadingGatewayProfiles,
            )}
            size="xl"
          />
        </Bullseye>
      )}
    </PageSection>
  );
}
