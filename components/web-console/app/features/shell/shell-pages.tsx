import {
  Button,
  Card,
  CardBody,
  CardFooter,
  CardTitle,
  Content,
  EmptyState,
  EmptyStateBody,
  EmptyStateVariant,
  Grid,
  GridItem,
  PageSection,
  Title,
} from "@patternfly/react-core";
import type { MessageDescriptor } from "react-intl";
import { FormattedMessage } from "react-intl";
import { Link } from "react-router";

import { messages } from "../../i18n/messages";

interface ScaffoldPageProps {
  description: MessageDescriptor;
  emptyBody: MessageDescriptor;
  emptyTitle: MessageDescriptor;
  title: MessageDescriptor;
}

/**
 * Temporary route content used while the resource API contract is settling.
 * It gives every shell destination a unique, localizable purpose and complete
 * empty state without introducing a second page-layout abstraction.
 */
function ScaffoldPage({
  description,
  emptyBody,
  emptyTitle,
  title,
}: ScaffoldPageProps) {
  return (
    <>
      <PageSection hasBodyWrapper={false}>
        <Content>
          <Title headingLevel="h1" size="2xl">
            <FormattedMessage {...title} />
          </Title>
          <p>
            <FormattedMessage {...description} />
          </p>
        </Content>
      </PageSection>
      <PageSection hasBodyWrapper={false} isFilled variant="secondary">
        <EmptyState
          headingLevel="h2"
          titleText={<FormattedMessage {...emptyTitle} />}
          variant={EmptyStateVariant.lg}
        >
          <EmptyStateBody>
            <FormattedMessage {...emptyBody} />
          </EmptyStateBody>
        </EmptyState>
      </PageSection>
    </>
  );
}

export function OverviewPage() {
  return (
    <>
      <PageSection hasBodyWrapper={false}>
        <Content>
          <Title headingLevel="h1" size="2xl">
            <FormattedMessage {...messages.overview} />
          </Title>
          <p>
            <FormattedMessage {...messages.overviewDescription} />
          </p>
        </Content>
      </PageSection>
      <PageSection hasBodyWrapper={false} isFilled variant="secondary">
        <Grid hasGutter>
          <GridItem md={6} span={12} xl={4}>
            <Card isFullHeight>
              <CardTitle>
                <Title headingLevel="h2" size="lg">
                  <FormattedMessage {...messages.sectors} />
                </Title>
              </CardTitle>
              <CardBody>
                <FormattedMessage {...messages.sectorsCardBody} />
              </CardBody>
              <CardFooter>
                <Button
                  component={Link}
                  isInline
                  variant="link"
                  {...{ to: "/fleets" }}
                >
                  <FormattedMessage {...messages.viewSectors} />
                </Button>
              </CardFooter>
            </Card>
          </GridItem>
          <GridItem md={6} span={12} xl={4}>
            <Card isFullHeight>
              <CardTitle>
                <Title headingLevel="h2" size="lg">
                  <FormattedMessage {...messages.gateways} />
                </Title>
              </CardTitle>
              <CardBody>
                <FormattedMessage {...messages.gatewaysCardBody} />
              </CardBody>
            </Card>
          </GridItem>
          <GridItem md={6} span={12} xl={4}>
            <Card isFullHeight>
              <CardTitle>
                <Title headingLevel="h2" size="lg">
                  <FormattedMessage {...messages.platformConnections} />
                </Title>
              </CardTitle>
              <CardBody>
                <FormattedMessage {...messages.platformConnectionsCardBody} />
              </CardBody>
            </Card>
          </GridItem>
        </Grid>
      </PageSection>
    </>
  );
}

export function SectorsPage() {
  return (
    <ScaffoldPage
      description={messages.sectorsDescription}
      emptyBody={messages.sectorsEmptyBody}
      emptyTitle={messages.sectorsEmptyTitle}
      title={messages.sectors}
    />
  );
}

export function SectorPage() {
  return (
    <ScaffoldPage
      description={messages.sectorDescription}
      emptyBody={messages.sectorEmptyBody}
      emptyTitle={messages.sectorEmptyTitle}
      title={messages.sectorOverview}
    />
  );
}

export function GatewaysPage() {
  return (
    <ScaffoldPage
      description={messages.gatewaysDescription}
      emptyBody={messages.gatewaysEmptyBody}
      emptyTitle={messages.gatewaysEmptyTitle}
      title={messages.gateways}
    />
  );
}

export function GatewayPage() {
  return (
    <ScaffoldPage
      description={messages.gatewayDescription}
      emptyBody={messages.gatewayEmptyBody}
      emptyTitle={messages.gatewayEmptyTitle}
      title={messages.gatewayDetails}
    />
  );
}

export function SettingsPage() {
  return (
    <ScaffoldPage
      description={messages.settingsDescription}
      emptyBody={messages.settingsEmptyBody}
      emptyTitle={messages.settingsEmptyTitle}
      title={messages.settings}
    />
  );
}

export function ClientsPage() {
  return (
    <ScaffoldPage
      description={messages.clientsDescription}
      emptyBody={messages.clientsEmptyBody}
      emptyTitle={messages.clientsEmptyTitle}
      title={messages.clients}
    />
  );
}

export function KeysPage() {
  return (
    <ScaffoldPage
      description={messages.keysDescription}
      emptyBody={messages.keysEmptyBody}
      emptyTitle={messages.keysEmptyTitle}
      title={messages.keys}
    />
  );
}
