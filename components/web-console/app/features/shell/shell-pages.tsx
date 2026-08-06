import {
  Alert,
  AlertActionCloseButton,
  AlertGroup,
  Button,
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
import { useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { FormattedMessage, useIntl } from "react-intl";
import { Link, useLocation, useNavigate, useParams } from "react-router";

import {
  gatewayStatusColor,
  type GatewayConnection,
} from "../gateways/gateway-connections";
import {
  GatewayCliCopy,
  GatewayDetailHeader,
  GatewayEndpointCopy,
} from "../gateways/gateway-detail-header";
import {
  gatewayQueryKey,
  getGateway,
  listGatewayConnections,
  toGatewayConnection,
  type GatewayRecord,
} from "../gateways/gateway-data";
import { GatewayLoadState } from "../gateways/gateway-load-state";
import { GatewayRowActions } from "../gateways/gateway-row-actions";
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

interface GatewaysPageProps {
  gateways?: readonly GatewayConnection[];
  onRefresh?: () => unknown;
}

function GatewaySuccessAlerts({
  deletedGatewayName,
  onDismissDeleted,
  onDismissRenamed,
  renamedGatewayName,
}: {
  deletedGatewayName?: string;
  onDismissDeleted?: () => void;
  onDismissRenamed: () => void;
  renamedGatewayName?: string;
}) {
  const intl = useIntl();

  if (!deletedGatewayName && !renamedGatewayName) {
    return null;
  }

  return (
    <AlertGroup
      aria-label={intl.formatMessage(messages.notifications)}
      isLiveRegion
      isToast
    >
      {renamedGatewayName ? (
        <Alert
          actionClose={
            <AlertActionCloseButton
              aria-label={intl.formatMessage(messages.close)}
              onClose={onDismissRenamed}
            />
          }
          onTimeout={onDismissRenamed}
          timeout={8000}
          title={intl.formatMessage(messages.gatewayRenamed, {
            gatewayName: renamedGatewayName,
          })}
          variant="success"
        />
      ) : null}
      {deletedGatewayName ? (
        <Alert
          actionClose={
            <AlertActionCloseButton
              aria-label={intl.formatMessage(messages.close)}
              onClose={() => {
                onDismissDeleted?.();
              }}
            />
          }
          onTimeout={() => {
            onDismissDeleted?.();
          }}
          timeout={8000}
          title={intl.formatMessage(messages.gatewayDeleted, {
            gatewayName: deletedGatewayName,
          })}
          variant="success"
        />
      ) : null}
    </AlertGroup>
  );
}

export function GatewaysPage({ gateways, onRefresh }: GatewaysPageProps = {}) {
  const intl = useIntl();
  const location = useLocation();
  const navigate = useNavigate();
  const routedDeletedGatewayName =
    typeof (location.state as { deletedGatewayName?: unknown } | null)
      ?.deletedGatewayName === "string"
      ? (location.state as { deletedGatewayName: string }).deletedGatewayName
      : undefined;
  const [deletedGatewayName, setDeletedGatewayName] = useState(
    routedDeletedGatewayName,
  );
  const [renamedGatewayName, setRenamedGatewayName] = useState<string>();
  useEffect(() => {
    if (!routedDeletedGatewayName) {
      return;
    }
    void navigate(location.pathname, { replace: true, state: null });
  }, [location.pathname, navigate, routedDeletedGatewayName]);
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
        <Link to={`/gateways/${encodeURIComponent(gateway.id)}`}>
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
      <GatewaySuccessAlerts
        deletedGatewayName={deletedGatewayName}
        onDismissDeleted={() => {
          setDeletedGatewayName(undefined);
        }}
        onDismissRenamed={() => {
          setRenamedGatewayName(undefined);
        }}
        renamedGatewayName={renamedGatewayName}
      />
      <PageSection hasBodyWrapper={false}>
        <Flex
          alignItems={{ default: "alignItemsFlexStart" }}
          justifyContent={{ default: "justifyContentSpaceBetween" }}
        >
          <FlexItem>
            <Title headingLevel="h1" size="2xl">
              <FormattedMessage {...messages.gateways} />
            </Title>
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
            emptyBody: intl.formatMessage(messages.gatewaysEmptyBody),
            emptyTitle: intl.formatMessage(messages.gatewaysEmptyTitle),
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
              {...{ to: "/gateways/new" }}
            >
              <FormattedMessage {...messages.provisionGateway} />
            </Button>
          }
          renderRowAction={(gateway) => (
            <GatewayRowActions
              gateway={gateway}
              onDeleted={() => {
                setDeletedGatewayName(gateway.name);
              }}
              onRenamed={setRenamedGatewayName}
            />
          )}
          rows={visibleGateways ?? []}
        />
      </PageSection>
    </>
  );
}

interface GatewayPageProps {
  gateway?: GatewayRecord;
}

export function GatewayPage({ gateway }: GatewayPageProps = {}) {
  const { gatewayId = "" } = useParams();
  const navigate = useNavigate();
  const [renamedGatewayName, setRenamedGatewayName] = useState<string>();
  const gatewayQuery = useQuery({
    enabled: gateway === undefined && gatewayId.length > 0,
    queryFn: ({ signal }) => getGateway(gatewayId, signal),
    queryKey: gatewayQueryKey(gatewayId),
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
      <GatewaySuccessAlerts
        onDismissRenamed={() => {
          setRenamedGatewayName(undefined);
        }}
        renamedGatewayName={renamedGatewayName}
      />
      <PageSection hasBodyWrapper={false}>
        <GatewayDetailHeader
          description={<FormattedMessage {...messages.gatewayDescription} />}
          gateway={connection}
          onDeleted={() => {
            void navigate("/", {
              state: { deletedGatewayName: connection.name },
            });
          }}
          onRenamed={setRenamedGatewayName}
        />
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
              <GatewayEndpointCopy gateway={connection} />
            </DescriptionListDescription>
          </DescriptionListGroup>
          <DescriptionListGroup>
            <DescriptionListTerm>
              <FormattedMessage {...messages.cliConnection} />
            </DescriptionListTerm>
            <DescriptionListDescription>
              <GatewayCliCopy gateway={connection} />
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
