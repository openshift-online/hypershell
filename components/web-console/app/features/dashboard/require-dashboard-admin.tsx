import {
  EmptyState,
  EmptyStateBody,
  EmptyStateVariant,
  PageSection,
  Spinner,
} from "@patternfly/react-core";
import { FormattedMessage, useIntl } from "react-intl";

import { messages } from "../../i18n/messages";
import { hasDashboardAdminRole } from "../../lib/session-roles";
import { useSession } from "../shell/use-session";

function DashboardAccessGate({
  children,
  requireAdmin,
}: Readonly<{
  children: React.ReactNode;
  requireAdmin: boolean;
}>) {
  const intl = useIntl();
  const { data: session, isLoading } = useSession();

  if (isLoading) {
    return (
      <PageSection hasBodyWrapper={false} isFilled>
        <Spinner
          aria-label={intl.formatMessage(messages.sessionLoadingLabel)}
        />
      </PageSection>
    );
  }

  const accessDenied = (
    <PageSection hasBodyWrapper={false} isFilled>
      <EmptyState
        variant={EmptyStateVariant.full}
        titleText={
          <FormattedMessage {...messages.dashboardAccessDeniedTitle} />
        }
        headingLevel="h1"
      >
        <EmptyStateBody>
          <FormattedMessage {...messages.dashboardAccessDeniedBody} />
        </EmptyStateBody>
      </EmptyState>
    </PageSection>
  );

  if (session?.authEnabled && !session.authenticated) {
    return accessDenied;
  }

  if (
    requireAdmin &&
    session?.authenticated &&
    !hasDashboardAdminRole(session.roles)
  ) {
    return accessDenied;
  }

  return children;
}

export function RequireAuthenticatedDashboard({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <DashboardAccessGate requireAdmin={false}>{children}</DashboardAccessGate>
  );
}

export function RequireDashboardAdmin({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return <DashboardAccessGate requireAdmin>{children}</DashboardAccessGate>;
}
