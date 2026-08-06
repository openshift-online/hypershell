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
  Label,
  PageSection,
  Title,
} from "@patternfly/react-core";
import type { MessageDescriptor } from "react-intl";
import { FormattedMessage, useIntl } from "react-intl";
import { Link } from "react-router";

import {
  availableClusterOptions,
  getGatewayProvisionPath,
  type ClusterOption,
} from "../clusters/cluster-options";
import {
  previewGateways,
  type GatewayConnection,
} from "../gateways/gateway-connections";
import {
  ResourceTable,
  type ResourceTableColumn,
} from "../shared/resource-table";
import { messages } from "../../i18n/messages";
import styles from "./shell-pages.module.css";

const primaryLinkStyle: React.CSSProperties = {
  color: "var(--pf-v6-c-button--m-primary--Color)",
  textDecoration: "none",
};

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

export function AdminOverviewPage() {
  return (
    <>
      <PageSection hasBodyWrapper={false}>
        <Content>
          <Title headingLevel="h1" size="2xl">
            <FormattedMessage {...messages.administration} />
          </Title>
          <p>
            <FormattedMessage {...messages.adminOverviewDescription} />
          </p>
        </Content>
      </PageSection>
      <PageSection hasBodyWrapper={false} isFilled variant="secondary">
        <Grid hasGutter>
          <GridItem md={6} span={12} xl={4}>
            <Card isFullHeight>
              <CardTitle>
                <Title headingLevel="h2" size="lg">
                  <FormattedMessage {...messages.clusters} />
                </Title>
              </CardTitle>
              <CardBody>
                <FormattedMessage {...messages.clustersCardBody} />
              </CardBody>
              <CardFooter>
                <Button
                  component={Link}
                  isInline
                  variant="link"
                  {...{ to: "/admin/clusters" }}
                >
                  <FormattedMessage {...messages.viewClusters} />
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
                <FormattedMessage {...messages.adminGatewaysCardBody} />
              </CardBody>
              <CardFooter>
                <Button
                  component={Link}
                  isInline
                  variant="link"
                  {...{ to: "/admin/gateways" }}
                >
                  <FormattedMessage {...messages.viewAdminGateways} />
                </Button>
              </CardFooter>
            </Card>
          </GridItem>
        </Grid>
      </PageSection>
    </>
  );
}

interface AdminGatewaysPageProps {
  gateways?: readonly GatewayConnection[];
}

export function AdminGatewaysPage({
  gateways = previewGateways,
}: AdminGatewaysPageProps = {}) {
  const intl = useIntl();
  const columns: readonly ResourceTableColumn<GatewayConnection>[] = [
    {
      getSortValue: ({ name }) => name,
      id: "name",
      label: intl.formatMessage(messages.gatewayName),
      render: (gateway) => (
        <Link to={`/admin/gateways/${encodeURIComponent(gateway.id)}`}>
          {gateway.name}
        </Link>
      ),
      width: 25,
    },
    {
      getSortValue: ({ status }) => status,
      id: "status",
      label: intl.formatMessage(messages.status),
      render: ({ status }) => (
        <Label color="green" isCompact>
          {status}
        </Label>
      ),
      width: 15,
    },
    {
      getSortValue: ({ endpoint }) => endpoint,
      id: "endpoint",
      label: intl.formatMessage(messages.gatewayEndpoint),
      render: ({ endpoint }) => (
        <span className={styles.endpoint}>{endpoint}</span>
      ),
    },
  ];

  return (
    <>
      <PageSection hasBodyWrapper={false}>
        <Content>
          <Title headingLevel="h1" size="2xl">
            <FormattedMessage {...messages.gateways} />
          </Title>
          <p>
            <FormattedMessage {...messages.adminGatewaysDescription} />
          </p>
        </Content>
      </PageSection>
      <PageSection hasBodyWrapper={false} isFilled>
        <ResourceTable
          ariaLabel={intl.formatMessage(messages.gateways)}
          columns={columns}
          getRowKey={({ id }) => id}
          id="gateways"
          labels={{
            actions: intl.formatMessage(messages.actions),
            clearFilters: intl.formatMessage(messages.clearFilters),
            emptyBody: intl.formatMessage(messages.adminGatewaysEmptyBody),
            emptyTitle: intl.formatMessage(messages.adminGatewaysEmptyTitle),
            noResultsBody: intl.formatMessage(messages.noMatchingGatewaysBody),
            noResultsTitle: intl.formatMessage(messages.noMatchingGateways),
            resultsCountContext: intl.formatMessage(messages.results),
            searchAriaLabel: intl.formatMessage(messages.filterGateways),
            searchPlaceholder: intl.formatMessage(messages.filterGateways),
          }}
          primaryAction={
            <Button
              component={Link}
              style={primaryLinkStyle}
              variant="primary"
              {...{ to: "/admin/gateways/new" }}
            >
              <FormattedMessage {...messages.provisionGateway} />
            </Button>
          }
          renderRowAction={(gateway) => (
            <Button
              component={Link}
              variant="secondary"
              {...{
                to: `/admin/gateways/${encodeURIComponent(gateway.id)}`,
              }}
            >
              <FormattedMessage {...messages.viewGatewayDetails} />
            </Button>
          )}
          rows={gateways}
        />
      </PageSection>
    </>
  );
}

export function AdminGatewayPage() {
  return (
    <ScaffoldPage
      description={messages.adminGatewayDescription}
      emptyBody={messages.adminGatewayEmptyBody}
      emptyTitle={messages.adminGatewayEmptyTitle}
      title={messages.adminGatewayDetails}
    />
  );
}

interface AdminClustersPageProps {
  clusters?: readonly ClusterOption[];
}

export function AdminClustersPage({
  clusters = availableClusterOptions,
}: AdminClustersPageProps = {}) {
  const intl = useIntl();
  const columns: readonly ResourceTableColumn<ClusterOption>[] = [
    {
      getSortValue: ({ name }) => name,
      id: "name",
      label: intl.formatMessage(messages.clusterNameColumn),
      render: ({ name }) => <strong>{name}</strong>,
      width: 30,
    },
    {
      getSortValue: ({ description }) => description,
      id: "description",
      label: intl.formatMessage(messages.clusterDescriptionColumn),
      render: ({ description }) => description,
    },
  ];

  return (
    <>
      <PageSection hasBodyWrapper={false}>
        <Content>
          <Title headingLevel="h1" size="2xl">
            <FormattedMessage {...messages.clusters} />
          </Title>
          <p>
            <FormattedMessage {...messages.clustersDescription} />
          </p>
        </Content>
      </PageSection>
      <PageSection hasBodyWrapper={false} isFilled>
        <ResourceTable
          ariaLabel={intl.formatMessage(messages.clusters)}
          columns={columns}
          getRowKey={({ id }) => id}
          id="clusters"
          labels={{
            actions: intl.formatMessage(messages.clusterActionsColumn),
            clearFilters: intl.formatMessage(messages.clearFilters),
            emptyBody: intl.formatMessage(messages.noClustersAvailableBody),
            emptyTitle: intl.formatMessage(messages.noClustersAvailable),
            noResultsBody: intl.formatMessage(messages.noMatchingClustersBody),
            noResultsTitle: intl.formatMessage(messages.noMatchingClusters),
            resultsCountContext: intl.formatMessage(messages.results),
            searchAriaLabel: intl.formatMessage(messages.filterClusters),
            searchPlaceholder: intl.formatMessage(messages.filterClusters),
          }}
          renderRowAction={(cluster) => (
            <Button
              component={Link}
              style={primaryLinkStyle}
              variant="primary"
              {...{ to: getGatewayProvisionPath(cluster.id) }}
            >
              <FormattedMessage {...messages.provisionGateway} />
            </Button>
          )}
          rows={clusters}
        />
      </PageSection>
    </>
  );
}
