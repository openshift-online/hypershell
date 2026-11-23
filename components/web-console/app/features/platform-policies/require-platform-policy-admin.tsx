import {
  Button,
  EmptyState,
  EmptyStateActions,
  EmptyStateBody,
  EmptyStateFooter,
  EmptyStateVariant,
  PageSection,
  Spinner,
} from "@patternfly/react-core";
import { LockIcon } from "@patternfly/react-icons";
import { useIntl } from "react-intl";

import { platformPolicyAccessMessages as messages } from "./platform-policy-access-messages";
import { usePlatformPolicyAccess } from "./use-platform-policy-access";

/**
 * Renders admin policy routes only for an authorized caller; a direct route
 * renders the standard not-authorized state instead of policy data
 * (WEB-PADM-01.01). The API server remains the authorization boundary.
 */
export function RequirePlatformPolicyAdmin({
  children,
}: Readonly<{ children: React.ReactNode }>): React.ReactNode {
  const intl = useIntl();
  const access = usePlatformPolicyAccess();

  if (access.isPending) {
    return (
      <PageSection hasBodyWrapper={false} isFilled>
        <Spinner aria-label={intl.formatMessage(messages.checkingAccess)} />
      </PageSection>
    );
  }
  if (access.isError) {
    return (
      <PageSection hasBodyWrapper={false} isFilled>
        <EmptyState
          headingLevel="h1"
          titleText={intl.formatMessage(messages.accessErrorTitle)}
          variant={EmptyStateVariant.full}
        >
          <EmptyStateFooter>
            <EmptyStateActions>
              <Button onClick={() => void access.refetch()} variant="primary">
                {intl.formatMessage(messages.retry)}
              </Button>
            </EmptyStateActions>
          </EmptyStateFooter>
        </EmptyState>
      </PageSection>
    );
  }
  if (!access.isAuthorized) {
    return (
      <PageSection hasBodyWrapper={false} isFilled>
        <EmptyState
          headingLevel="h1"
          icon={LockIcon}
          titleText={intl.formatMessage(messages.notAuthorizedTitle)}
          variant={EmptyStateVariant.full}
        >
          <EmptyStateBody>
            {intl.formatMessage(messages.notAuthorizedBody)}
          </EmptyStateBody>
        </EmptyState>
      </PageSection>
    );
  }
  return children;
}
