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
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { FormattedMessage, useIntl } from "react-intl";

import {
  defaultGatewayListRequest,
  type GatewayListRequest,
  type GatewayRecord,
  type GatewaySortField,
} from "../application/gateway-types";
import { useGatewayLink, useGatewayUi } from "../gateway-ui-provider";
import {
  buildGatewayAddCommand,
  gatewayStatusColor,
  type GatewayConnection,
} from "../gateways/gateway-connections";
import {
  GatewayCliCopy,
  GatewayDetailHeader,
  GatewayEndpointCopy,
} from "../gateways/gateway-detail-header";
import {
  gatewayListQueryKey,
  gatewayPlacementBatchQueryKey,
  gatewayPlacementDetailQueryKey,
  gatewayPlacementStaleMilliseconds,
  gatewayQueryKey,
  gatewaySearchDebounceMilliseconds,
  toGatewayConnection,
} from "../gateways/gateway-data";
import { GatewayLoadState } from "../gateways/gateway-load-state";
import { GatewayRowActions } from "../gateways/gateway-row-actions";
import {
  ResourceTable,
  type ResourceTableColumn,
  type ResourceTableState,
  type ResourceTableStateChangeReason,
} from "../shared/resource-table";
import { ResourceRefreshButton } from "../shared/resource-refresh-button";
import { useDebouncedValue } from "../shared/use-debounced-value";
import { messages } from "../messages";
import styles from "./gateway-pages.module.css";

const primaryLinkStyle: React.CSSProperties = {
  color: "var(--pf-v6-c-button--m-primary--Color)",
  textDecoration: "none",
};

export interface GatewaysPageProps {
  collectionState?: GatewayListRequest;
  deletedGatewayName?: string;
  gateways?: readonly GatewayConnection[];
  onCollectionStateChange?: (
    state: GatewayListRequest,
    reason: ResourceTableStateChangeReason,
  ) => void;
  onDismissDeletedGateway?: () => void;
  onRefresh?: () => unknown;
}

function isGatewaySortField(value: string): value is GatewaySortField {
  return ["cluster", "endpoint", "name", "status"].includes(value);
}

function GatewayDetailLink({ gateway }: { gateway: GatewayConnection }) {
  const { navigation } = useGatewayUi();
  const link = useGatewayLink(navigation.detailHref(gateway.id));

  return <a {...link}>{gateway.name}</a>;
}

function GatewayDetailClusterName({ gateway }: { gateway: GatewayConnection }) {
  const intl = useIntl();
  const { gateways } = useGatewayUi();
  const clusterId = gateway.clusterId ?? "";
  const placementQuery = useQuery({
    enabled: clusterId.length > 0,
    queryFn: ({ signal }) => gateways.getGatewayPlacement(clusterId, signal),
    queryKey: gatewayPlacementDetailQueryKey(clusterId),
    staleTime: gatewayPlacementStaleMilliseconds,
  });

  if (!clusterId) {
    return gateway.clusterName;
  }
  if (placementQuery.isPending) {
    return (
      <span role="status">
        {intl.formatMessage(messages.loadingClusterName)}
      </span>
    );
  }
  if (placementQuery.isError) {
    return intl.formatMessage(messages.notAvailable);
  }
  return placementQuery.data.name;
}

function GatewayCollectionClusterName({
  gateway,
  isLoading,
  placementNames,
}: {
  gateway: GatewayConnection;
  isLoading: boolean;
  placementNames: ReadonlyMap<string, string>;
}) {
  const intl = useIntl();
  const clusterId = gateway.clusterId ?? "";

  if (!clusterId || gateway.clusterName.trim()) {
    return gateway.clusterName;
  }
  if (isLoading) {
    return (
      <span role="status">
        {intl.formatMessage(messages.loadingClusterName)}
      </span>
    );
  }
  return (
    placementNames.get(clusterId) ?? intl.formatMessage(messages.notAvailable)
  );
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

export function GatewaysPage({
  collectionState,
  deletedGatewayName: initialDeletedGatewayName,
  gateways,
  onCollectionStateChange,
  onDismissDeletedGateway,
  onRefresh,
}: GatewaysPageProps = {}) {
  const intl = useIntl();
  const { gateways: gatewayOperations, navigation } = useGatewayUi();
  const createLink = useGatewayLink(navigation.createHref);
  const [deletedGatewayName, setDeletedGatewayName] = useState<string>();
  const [isInitialDeletionDismissed, setIsInitialDeletionDismissed] =
    useState(false);
  const [renamedGatewayName, setRenamedGatewayName] = useState<string>();
  const [localCollectionState, setLocalCollectionState] =
    useState<GatewayListRequest>({ ...defaultGatewayListRequest });
  const currentCollectionState = collectionState ?? localCollectionState;
  const debouncedGatewaySearch = useDebouncedValue(
    currentCollectionState.search.trim(),
    gatewaySearchDebounceMilliseconds,
  );
  const gatewayRequest: GatewayListRequest = {
    ...currentCollectionState,
    search: debouncedGatewaySearch,
  };
  const gatewayQuery = useQuery({
    enabled: gateways === undefined,
    placeholderData: keepPreviousData,
    queryFn: async ({ signal }) => {
      const result = await gatewayOperations.listGateways(
        gatewayRequest,
        signal,
      );
      return {
        ...result,
        items: result.items.map((gateway) =>
          toGatewayConnection(gateway, intl.formatMessage(messages.hubCluster)),
        ),
      };
    },
    queryKey: gatewayListQueryKey(gatewayRequest),
  });
  const visiblePage = gateways
    ? {
        items: gateways,
        page: currentCollectionState.page,
        size: currentCollectionState.size,
        total: gateways.length,
      }
    : gatewayQuery.data;
  const placementClusterIds = [
    ...new Set(
      (visiblePage?.items ?? [])
        .filter(
          (gateway) =>
            Boolean(gateway.clusterId) && !gateway.clusterName.trim(),
        )
        .map((gateway) => gateway.clusterId ?? ""),
    ),
  ].sort();
  const placementsQuery = useQuery({
    enabled: placementClusterIds.length > 0,
    queryFn: ({ signal }) =>
      gatewayOperations.getGatewayPlacements(placementClusterIds, signal),
    queryKey: gatewayPlacementBatchQueryKey(placementClusterIds),
    staleTime: gatewayPlacementStaleMilliseconds,
  });
  const placementNames = new Map(
    placementsQuery.data?.map(({ id, name }) => [id, name]),
  );
  const tableState: ResourceTableState = {
    page: currentCollectionState.page,
    query: currentCollectionState.search,
    sortColumnId: currentCollectionState.sortField,
    sortDirection: currentCollectionState.sortDirection,
  };
  const changeTableState = (
    nextState: ResourceTableState,
    reason: ResourceTableStateChangeReason,
  ) => {
    const nextCollectionState: GatewayListRequest = {
      page: nextState.page,
      search: nextState.query,
      size: currentCollectionState.size,
      sortDirection: nextState.sortDirection,
      sortField: isGatewaySortField(nextState.sortColumnId)
        ? nextState.sortColumnId
        : "name",
    };
    if (onCollectionStateChange) {
      onCollectionStateChange(nextCollectionState, reason);
    } else {
      setLocalCollectionState(nextCollectionState);
    }
  };
  const columns: readonly ResourceTableColumn<GatewayConnection>[] = [
    {
      id: "name",
      label: intl.formatMessage(messages.gatewayName),
      render: (gateway) => <GatewayDetailLink gateway={gateway} />,
      width: 25,
    },
    {
      id: "cluster",
      label: intl.formatMessage(messages.cluster),
      render: (gateway) => (
        <GatewayCollectionClusterName
          gateway={gateway}
          isLoading={placementsQuery.isPending}
          placementNames={placementNames}
        />
      ),
      width: 20,
    },
    {
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
      id: "endpoint",
      label: intl.formatMessage(messages.gatewayEndpoint),
      render: ({ endpoint }) => (
        <span className={styles.endpoint}>
          {endpoint ?? intl.formatMessage(messages.notAvailable)}
        </span>
      ),
    },
  ];

  if (!visiblePage && gatewayQuery.isPending) {
    return <GatewayLoadState />;
  }
  if (!visiblePage && gatewayQuery.isError) {
    return <GatewayLoadState isError />;
  }

  return (
    <>
      <GatewaySuccessAlerts
        deletedGatewayName={
          deletedGatewayName ??
          (isInitialDeletionDismissed ? undefined : initialDeletedGatewayName)
        }
        onDismissDeleted={() => {
          setDeletedGatewayName(undefined);
          setIsInitialDeletionDismissed(true);
          onDismissDeletedGateway?.();
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
          itemCount={visiblePage?.total ?? 0}
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
              component="a"
              style={primaryLinkStyle}
              variant="primary"
              {...createLink}
            >
              <FormattedMessage {...messages.provisionGateway} />
            </Button>
          }
          onStateChange={changeTableState}
          pageSize={currentCollectionState.size}
          renderRowAction={(gateway) => (
            <GatewayRowActions
              gateway={gateway}
              onDeleted={() => {
                setDeletedGatewayName(gateway.name);
              }}
              onRenamed={setRenamedGatewayName}
            />
          )}
          rows={visiblePage?.items ?? []}
          state={tableState}
        />
      </PageSection>
    </>
  );
}

export interface GatewayPageProps {
  gateway?: GatewayRecord;
  gatewayId: string;
  onDeleted?: (gatewayName: string) => Promise<void> | void;
}

export function GatewayPage({
  gateway,
  gatewayId,
  onDeleted,
}: GatewayPageProps) {
  const intl = useIntl();
  const { gateways, navigation } = useGatewayUi();
  const [renamedGatewayName, setRenamedGatewayName] = useState<string>();
  const gatewayQuery = useQuery({
    enabled: gateway === undefined && gatewayId.length > 0,
    queryFn: ({ signal }) => gateways.getGateway(gatewayId, signal),
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

  const connection = toGatewayConnection(
    visibleGateway,
    intl.formatMessage(messages.hubCluster),
  );

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
            if (onDeleted) {
              void onDeleted(connection.name);
            } else {
              void navigation.navigate(navigation.collectionHref, {
                state: { deletedGatewayName: connection.name },
              });
            }
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
              <FormattedMessage {...messages.cluster} />
            </DescriptionListTerm>
            <DescriptionListDescription>
              <GatewayDetailClusterName gateway={connection} />
            </DescriptionListDescription>
          </DescriptionListGroup>
          <DescriptionListGroup>
            <DescriptionListTerm>
              <FormattedMessage {...messages.gatewayEndpoint} />
            </DescriptionListTerm>
            <DescriptionListDescription>
              {connection.endpoint ? (
                <GatewayEndpointCopy gateway={connection} />
              ) : (
                <FormattedMessage {...messages.notAvailable} />
              )}
            </DescriptionListDescription>
          </DescriptionListGroup>
          <DescriptionListGroup>
            <DescriptionListTerm>
              <FormattedMessage {...messages.cliConnection} />
            </DescriptionListTerm>
            <DescriptionListDescription>
              {buildGatewayAddCommand(connection) ? (
                <GatewayCliCopy gateway={connection} />
              ) : (
                <FormattedMessage {...messages.notAvailable} />
              )}
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
              {visibleGateway.releaseId}
            </DescriptionListDescription>
          </DescriptionListGroup>
          <DescriptionListGroup>
            <DescriptionListTerm>
              <FormattedMessage {...messages.managedDatabaseId} />
            </DescriptionListTerm>
            <DescriptionListDescription>
              {visibleGateway.databaseId}
            </DescriptionListDescription>
          </DescriptionListGroup>
        </DescriptionList>
      </PageSection>
    </>
  );
}
