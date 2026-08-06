import {
  Button,
  Content,
  DescriptionList,
  DescriptionListDescription,
  DescriptionListGroup,
  DescriptionListTerm,
  Flex,
  FlexItem,
  Label,
  PageSection,
  Title,
} from "@patternfly/react-core";
import type { Gateway } from "@openshift-online/hypershell-sdk";
import { useQuery } from "@tanstack/react-query";
import { FormattedMessage, useIntl } from "react-intl";
import { Link, useParams } from "react-router";

import {
  gatewayStatusColor,
  type GatewayConnection,
} from "../gateways/gateway-connections";
import {
  getGateway,
  listGatewayConnections,
  toGatewayConnection,
} from "../gateways/gateway-data";
import { GatewayLoadState } from "../gateways/gateway-load-state";
import {
  ResourceTable,
  type ResourceTableColumn,
} from "../shared/resource-table";
import { ResourceRefreshButton } from "../shared/resource-refresh-button";
import { messages } from "../../i18n/messages";
import styles from "./shell-pages.module.css";

const primaryLinkStyle: React.CSSProperties = {
  color: "var(--pf-v6-c-button--m-primary--Color)",
  textDecoration: "none",
};

interface AdminGatewaysPageProps {
  gateways?: readonly GatewayConnection[];
  onRefresh?: () => unknown;
}

export function AdminGatewaysPage({
  gateways,
  onRefresh,
}: AdminGatewaysPageProps = {}) {
  const intl = useIntl();
  const gatewayQuery = useQuery({
    enabled: gateways === undefined,
    queryFn: ({ signal }) => listGatewayConnections(signal),
    queryKey: ["gateways"],
  });
  const visibleGateways = gateways ?? gatewayQuery.data;
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
      getSortValue: ({ clusterName }) => clusterName,
      id: "cluster",
      label: intl.formatMessage(messages.cluster),
      render: ({ clusterName }) => clusterName,
      width: 20,
    },
    {
      getSortValue: ({ status }) => status,
      id: "status",
      label: intl.formatMessage(messages.status),
      render: ({ status }) => (
        <Label color={gatewayStatusColor(status)} isCompact>
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

  if (!visibleGateways && gatewayQuery.isPending) {
    return <GatewayLoadState />;
  }
  if (!visibleGateways && gatewayQuery.isError) {
    return <GatewayLoadState isError />;
  }

  return (
    <>
      <PageSection hasBodyWrapper={false}>
        <Flex
          alignItems={{ default: "alignItemsFlexStart" }}
          justifyContent={{ default: "justifyContentSpaceBetween" }}
        >
          <FlexItem>
            <Content>
              <Title headingLevel="h1" size="2xl">
                <FormattedMessage {...messages.gateways} />
              </Title>
              <p>
                <FormattedMessage {...messages.adminGatewaysDescription} />
              </p>
            </Content>
          </FlexItem>
          {gateways === undefined || onRefresh ? (
            <FlexItem>
              <ResourceRefreshButton
                ariaLabel={intl.formatMessage(messages.refreshGateways)}
                isRefreshing={gatewayQuery.isFetching}
                onRefresh={onRefresh ?? (() => gatewayQuery.refetch())}
              />
            </FlexItem>
          ) : null}
        </Flex>
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
          rows={visibleGateways ?? []}
        />
      </PageSection>
    </>
  );
}

interface AdminGatewayPageProps {
  gateway?: Gateway;
}

export function AdminGatewayPage({ gateway }: AdminGatewayPageProps = {}) {
  const { gatewayId = "" } = useParams();
  const gatewayQuery = useQuery({
    enabled: gateway === undefined && gatewayId.length > 0,
    queryFn: ({ signal }) => getGateway(gatewayId, signal),
    queryKey: ["gateways", gatewayId],
  });
  const visibleGateway = gateway ?? gatewayQuery.data;

  if (!visibleGateway && gatewayQuery.isPending) {
    return <GatewayLoadState />;
  }
  if (!visibleGateway && gatewayQuery.isError) {
    return <GatewayLoadState isError />;
  }
  if (!visibleGateway) {
    return <GatewayLoadState isError />;
  }

  const connection = toGatewayConnection(visibleGateway);

  return (
    <>
      <PageSection hasBodyWrapper={false}>
        <Content>
          <Title headingLevel="h1" size="2xl">
            {visibleGateway.name}
          </Title>
          <p>
            <FormattedMessage {...messages.adminGatewayDescription} />
          </p>
        </Content>
      </PageSection>
      <PageSection hasBodyWrapper={false} isFilled variant="secondary">
        <DescriptionList isHorizontal>
          <DescriptionListGroup>
            <DescriptionListTerm>
              <FormattedMessage {...messages.status} />
            </DescriptionListTerm>
            <DescriptionListDescription>
              {connection.status}
            </DescriptionListDescription>
          </DescriptionListGroup>
          <DescriptionListGroup>
            <DescriptionListTerm>
              <FormattedMessage {...messages.gatewayEndpoint} />
            </DescriptionListTerm>
            <DescriptionListDescription>
              {connection.endpoint}
            </DescriptionListDescription>
          </DescriptionListGroup>
          <DescriptionListGroup>
            <DescriptionListTerm>
              <FormattedMessage {...messages.namespace} />
            </DescriptionListTerm>
            <DescriptionListDescription>
              {visibleGateway.namespace}
            </DescriptionListDescription>
          </DescriptionListGroup>
          <DescriptionListGroup>
            <DescriptionListTerm>
              <FormattedMessage {...messages.gatewayReleaseId} />
            </DescriptionListTerm>
            <DescriptionListDescription>
              {visibleGateway.release_id}
            </DescriptionListDescription>
          </DescriptionListGroup>
          <DescriptionListGroup>
            <DescriptionListTerm>
              <FormattedMessage {...messages.managedDatabaseId} />
            </DescriptionListTerm>
            <DescriptionListDescription>
              {visibleGateway.database_id}
            </DescriptionListDescription>
          </DescriptionListGroup>
        </DescriptionList>
      </PageSection>
    </>
  );
}
