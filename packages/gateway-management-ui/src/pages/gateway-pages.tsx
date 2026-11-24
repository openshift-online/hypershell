import {
  Button,
  Flex,
  FlexItem,
  PageSection,
  Switch,
  Title,
  ToolbarItem,
} from "@patternfly/react-core";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { useCallback, useEffect, useState } from "react";
import { FormattedMessage, useIntl } from "react-intl";
import {
  defaultGatewayListRequest,
  gatewayListPageSizes,
  type GatewayListRequest,
  type GatewaySortField,
} from "../application/gateway-types";
import { useGatewayLink, useGatewayUi } from "../gateway-ui-provider";
import {
  GatewayBulkDeleteConfirmModal,
  GatewayBulkDeleteResultBanner,
  GatewayBulkSelectControl,
  GatewaySelectionCheckbox,
  isGatewayDeletable,
  useGatewayBulkDelete,
} from "../gateways/gateway-bulk-delete";
import { useConsoleWaitTracker } from "../gateways/console-wait-tracker";
import { type GatewayConnection } from "../gateways/gateway-connections";
import { GatewayConnectionCommandModal } from "../gateways/gateway-connection-command-modal";
import {
  gatewayListQueryKey,
  gatewayNeedsStatusPolling,
  gatewaySearchDebounceMilliseconds,
  gatewayStatusPollMilliseconds,
  toGatewayConnection,
} from "../gateways/gateway-data";
import { GatewayLoadState } from "../gateways/gateway-load-state";
import { GatewayRowActions } from "../gateways/gateway-row-actions";
import { GatewayStatus } from "../gateways/gateway-status";
import {
  ResourceTable,
  type ResourceTableColumn,
  type ResourceTableState,
  type ResourceTableStateChangeReason,
} from "../shared/resource-table";
import { ResourceRefreshButton } from "../shared/resource-refresh-button";
import { useDebouncedValue } from "../shared/use-debounced-value";
import { messages } from "../messages";
import { GatewayCreatedDate, GatewaySuccessAlerts } from "./gateway-page-parts";
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
    collectionState: GatewayListRequest,
    reason: ResourceTableStateChangeReason,
  ) => void;
  onDismissDeletedGateway?: () => void;
  onRefresh?: () => unknown;
}

function isGatewaySortField(value: string): value is GatewaySortField {
  return [
    "activeSandboxes",
    "cluster",
    "created",
    "endpoint",
    "name",
    "owner",
    "status",
    "version",
  ].includes(value);
}

function GatewayDetailLink({ gateway }: { gateway: GatewayConnection }) {
  const { navigation } = useGatewayUi();
  const link = useGatewayLink(navigation.detailHref(gateway.id));

  return <a {...link}>{gateway.name}</a>;
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
  const {
    currentUser,
    gateways: gatewayOperations,
    navigation,
  } = useGatewayUi();
  const trackConsoleWait = useConsoleWaitTracker();
  const createLink = useGatewayLink(navigation.createHref);
  const [deletedGatewayName, setDeletedGatewayName] = useState<string>();
  const [isInitialDeletionDismissed, setIsInitialDeletionDismissed] =
    useState(false);
  const [renamedGatewayName, setRenamedGatewayName] = useState<string>();
  const [localCollectionState, setLocalCollectionState] =
    useState<GatewayListRequest>({ ...defaultGatewayListRequest });
  const [connectionCommandGateway, setConnectionCommandGateway] =
    useState<GatewayConnection | null>(null);
  const currentCollectionState = collectionState ?? localCollectionState;
  const debouncedGatewaySearch = useDebouncedValue(
    currentCollectionState.search.trim(),
    gatewaySearchDebounceMilliseconds,
  );
  const gatewayRequest: GatewayListRequest = {
    ...currentCollectionState,
    search: debouncedGatewaySearch,
    ...(currentUser?.isPlatformAdmin ? { searchCreatedBy: true } : {}),
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
        items: result.items.map((gateway) => toGatewayConnection(gateway)),
        // Map before reducing: trackConsoleWait records each gateway's
        // console-wait start as a side effect, so every returned gateway must be
        // observed. some() short-circuits, which would leave later gateways
        // without a wait clock.
        shouldPollStatus: result.items
          .map((gateway) =>
            gatewayNeedsStatusPolling(
              gateway,
              trackConsoleWait(gateway.id, gateway),
            ),
          )
          .some(Boolean),
      };
    },
    queryKey: gatewayListQueryKey(gatewayRequest),
    refetchInterval: ({ state }) =>
      state.data?.shouldPollStatus ? gatewayStatusPollMilliseconds : false,
    refetchIntervalInBackground: false,
  });
  const visiblePage = gateways
    ? {
        items: gateways,
        page: currentCollectionState.page,
        size: currentCollectionState.size,
        total: gateways.length,
        // An externally supplied gateway list carries no server-computed
        // authorized-set existence flag, so the ownership switch stays hidden
        // unless it is already enabled (WEB-UIEX-07, WEB-API-01).
        hasOtherOwnedGateways: undefined as boolean | undefined,
      }
    : gatewayQuery.data;
  const tableState: ResourceTableState = {
    page: currentCollectionState.page,
    pageSize: currentCollectionState.size,
    query: currentCollectionState.search,
    sortColumnId: currentCollectionState.sortField,
    sortDirection: currentCollectionState.sortDirection,
  };
  const onlyMine = currentCollectionState.onlyMine ?? false;
  const applyCollectionState = (
    nextCollectionState: GatewayListRequest,
    reason: ResourceTableStateChangeReason,
  ) => {
    if (onCollectionStateChange) {
      onCollectionStateChange(nextCollectionState, reason);
    } else {
      setLocalCollectionState(nextCollectionState);
    }
  };
  // Clamp the page down once a refetch settles on a page past the last page of
  // results. A bulk delete that removes every row on a later page (WEB-UIEX-08)
  // leaves pagination pointing beyond the data; without this the table would
  // show an empty later page while earlier pages still have rows. Gating on
  // settled, non-empty results avoids clamping against the stale previous page
  // kept by keepPreviousData during the in-flight refetch or an empty filter.
  const totalGatewayCount = visiblePage?.total ?? 0;
  const lastGatewayPage = Math.max(
    1,
    Math.ceil(totalGatewayCount / currentCollectionState.size),
  );
  const pageExceedsResults =
    gateways === undefined &&
    !gatewayQuery.isFetching &&
    totalGatewayCount > 0 &&
    currentCollectionState.page > lastGatewayPage;
  // Uncontrolled: adjust own state during render, the React-recommended
  // alternative to a setState-in-effect. The clamp makes the condition false on
  // the next render, so it runs once and does not loop.
  if (pageExceedsResults && !onCollectionStateChange) {
    setLocalCollectionState((previous) => ({
      ...previous,
      page: lastGatewayPage,
    }));
  }
  // Controlled: notify the host after render. This syncs an external store (the
  // host owns the collection state), not local React state, so it does not
  // cascade local renders.
  useEffect(() => {
    if (pageExceedsResults && onCollectionStateChange) {
      onCollectionStateChange(
        { ...currentCollectionState, page: lastGatewayPage },
        "page",
      );
    }
  }, [
    currentCollectionState,
    lastGatewayPage,
    onCollectionStateChange,
    pageExceedsResults,
  ]);
  const changeTableState = (
    nextState: ResourceTableState,
    reason: ResourceTableStateChangeReason,
  ) => {
    applyCollectionState(
      {
        page: nextState.page,
        search: nextState.query,
        size: nextState.pageSize,
        sortDirection: nextState.sortDirection,
        sortField: isGatewaySortField(nextState.sortColumnId)
          ? nextState.sortColumnId
          : "name",
        // Preserve the ownership filter across search, sort, and pagination; it
        // is not part of the shared table state (WEB-UIEX-07).
        onlyMine,
      },
      reason,
    );
  };
  // Changing the ownership filter switch resets to the first page and composes
  // with the active search and sort (WEB-UIEX-07).
  const changeOwnershipFilter = (nextOnlyMine: boolean) => {
    applyCollectionState(
      { ...currentCollectionState, onlyMine: nextOnlyMine, page: 1 },
      "filter",
    );
  };
  // Show the switch only when the caller's full authorized list contains a
  // gateway owned by someone else, and keep it visible while enabled so the user
  // can restore the full list (WEB-UIEX-07). Server-computed over the authorized
  // set only (WEB-API-01, SEC-RBAC-10).
  // Bulk selection and bulk delete (WEB-UIEX-08). Selection scope is the current
  // page only; it clears whenever the collection scope changes so a hidden row
  // can never be deleted.
  const visibleItems = visiblePage?.items ?? [];
  const selectionScopeKey = [
    currentCollectionState.page,
    currentCollectionState.size,
    currentCollectionState.search,
    currentCollectionState.sortField,
    currentCollectionState.sortDirection,
    onlyMine,
  ].join("|");
  const [selectedIds, setSelectedIds] = useState<ReadonlySet<string>>(
    () => new Set(),
  );
  // Reset the selection during render when the collection scope changes, the
  // React-recommended alternative to a scope-keyed effect. A refetch within the
  // same scope keeps the selection so failed deletions stay selected for retry.
  const [trackedSelectionScope, setTrackedSelectionScope] =
    useState(selectionScopeKey);
  if (trackedSelectionScope !== selectionScopeKey) {
    setTrackedSelectionScope(selectionScopeKey);
    setSelectedIds(new Set());
  }
  const removeFromSelection = useCallback((ids: readonly string[]) => {
    if (ids.length === 0) {
      return;
    }
    setSelectedIds((previous) => {
      const next = new Set(previous);
      for (const id of ids) {
        next.delete(id);
      }
      return next;
    });
  }, []);
  const toggleRowSelection = useCallback(
    (gatewayId: string, selected: boolean) => {
      setSelectedIds((previous) => {
        const next = new Set(previous);
        if (selected) {
          next.add(gatewayId);
        } else {
          next.delete(gatewayId);
        }
        return next;
      });
    },
    [],
  );
  const bulkDelete = useGatewayBulkDelete({ onAccepted: removeFromSelection });
  // Eligibility is the authoritative delete capability only; it never bypasses
  // the server authorization boundary (WEB-UIEX-08). Selection reconciles
  // against the currently visible rows so a refetch drops removed gateways.
  const eligibleGateways = visibleItems.filter(isGatewayDeletable);
  const eligibleCount = eligibleGateways.length;
  const selectedGateways = eligibleGateways.filter((gateway) =>
    selectedIds.has(gateway.id),
  );
  const selectedCount = selectedGateways.length;
  const showBulkControls = visibleItems.length > 0;
  const selectEligiblePage = () => {
    setSelectedIds(new Set(eligibleGateways.map((gateway) => gateway.id)));
  };
  const clearSelection = () => {
    setSelectedIds(new Set());
  };
  const baseColumns: readonly ResourceTableColumn<GatewayConnection>[] = [
    {
      id: "name",
      label: intl.formatMessage(messages.gatewayName),
      render: (gateway) => <GatewayDetailLink gateway={gateway} />,
      width: 25,
    },
    {
      // The API orders this column directly; an unset (never observed)
      // count follows the database null-ordering convention and renders
      // the localized not-available fallback rather than a misleading zero.
      id: "activeSandboxes",
      label: intl.formatMessage(messages.sandboxes),
      render: ({ activeSandboxCount }) =>
        typeof activeSandboxCount === "number"
          ? String(activeSandboxCount)
          : intl.formatMessage(messages.notAvailable),
      width: 15,
    },
    {
      id: "status",
      label: intl.formatMessage(messages.status),
      render: ({ status }) => <GatewayStatus status={status} />,
      width: 15,
    },
    {
      // Running version, ordered server-side by the `version` sort field. No
      // chip on list rows (WEB-VER-01); the version appears here only as text.
      id: "version",
      label: intl.formatMessage(messages.version),
      render: ({ gatewayVersion }) =>
        gatewayVersion?.trim()
          ? gatewayVersion
          : intl.formatMessage(messages.versionPlaceholder),
      width: 10,
    },
    {
      id: "created",
      label: intl.formatMessage(messages.created),
      render: ({ createdAt }) => <GatewayCreatedDate createdAt={createdAt} />,
      width: 20,
    },
  ];

  const columns: readonly ResourceTableColumn<GatewayConnection>[] =
    currentUser?.isPlatformAdmin
      ? [
          ...baseColumns,
          {
            id: "owner",
            label: intl.formatMessage(messages.createdBy),
            render: ({ createdBy }) =>
              createdBy?.trim()
                ? createdBy
                : intl.formatMessage(messages.notAvailable),
            width: 15,
          },
        ]
      : baseColumns;

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
      <PageSection
        data-page-region="header"
        hasBodyWrapper={false}
        variant="default"
      >
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
      <PageSection
        className={styles.tableSection}
        data-page-region="body"
        hasBodyWrapper={false}
        isFilled
        variant="secondary"
      >
        <ResourceTable
          ariaLabel={intl.formatMessage(messages.gateways)}
          banner={
            bulkDelete.phase === "results" || bulkDelete.isRetrying ? (
              <GatewayBulkDeleteResultBanner
                isRetrying={bulkDelete.isRetrying}
                onDismiss={bulkDelete.dismissResults}
                onRetry={bulkDelete.retryFailed}
                outcomes={bulkDelete.outcomes}
              />
            ) : undefined
          }
          bulkActions={
            showBulkControls ? (
              <>
                <ToolbarItem>
                  <Button
                    aria-label={intl.formatMessage(
                      messages.bulkDeleteButtonAria,
                      { count: selectedCount },
                    )}
                    isDanger
                    isDisabled={selectedCount === 0 || bulkDelete.isBusy}
                    onClick={() => {
                      bulkDelete.request(
                        selectedGateways.map((gateway) => ({
                          id: gateway.id,
                          name: gateway.name,
                          ...(typeof gateway.activeSandboxCount === "number"
                            ? { activeSandboxCount: gateway.activeSandboxCount }
                            : {}),
                        })),
                      );
                    }}
                    variant="secondary"
                  >
                    <FormattedMessage {...messages.bulkDeleteButton} />
                  </Button>
                </ToolbarItem>
                {selectedCount > 0 ? (
                  <ToolbarItem>
                    <Button onClick={clearSelection} variant="link">
                      <FormattedMessage
                        {...messages.bulkDeleteClearSelection}
                      />
                    </Button>
                  </ToolbarItem>
                ) : null}
              </>
            ) : undefined
          }
          bulkSelect={
            showBulkControls ? (
              <GatewayBulkSelectControl
                eligibleCount={eligibleCount}
                onSelectNone={clearSelection}
                onSelectPage={selectEligiblePage}
                selectedCount={selectedCount}
              />
            ) : undefined
          }
          columns={columns}
          getRowKey={({ id }) => id}
          hasActiveFilters={
            onlyMine || currentCollectionState.search.trim().length > 0
          }
          id="gateways"
          itemCount={visiblePage?.total ?? 0}
          labels={{
            actions: intl.formatMessage(messages.actions),
            clearFilters: intl.formatMessage(messages.clearFilters),
            emptyBody: intl.formatMessage(messages.gatewaysEmptyBody),
            emptyTitle: intl.formatMessage(messages.gatewaysEmptyTitle),
            noResultsBody: intl.formatMessage(
              onlyMine
                ? messages.noOwnedGatewaysBody
                : messages.noMatchingGatewaysBody,
            ),
            noResultsTitle: intl.formatMessage(
              onlyMine
                ? messages.noOwnedGatewaysTitle
                : messages.noMatchingGateways,
            ),
            resultsCountContext: intl.formatMessage(messages.results),
            searchAriaLabel: intl.formatMessage(
              currentUser?.isPlatformAdmin
                ? messages.filterGatewaysAdmin
                : messages.filterGateways,
            ),
            searchPlaceholder: intl.formatMessage(
              currentUser?.isPlatformAdmin
                ? messages.filterGatewaysAdmin
                : messages.filterGateways,
            ),
          }}
          onClearFilters={() => {
            applyCollectionState(
              {
                ...currentCollectionState,
                onlyMine: false,
                page: 1,
                search: "",
              },
              "filter",
            );
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
          pageSizeOptions={gatewayListPageSizes}
          renderRowAction={(gateway) => (
            <GatewayRowActions
              gateway={gateway}
              onConnectionCommand={() => {
                setConnectionCommandGateway(gateway);
              }}
              onDeleted={() => {
                setDeletedGatewayName(gateway.name);
              }}
              onRenamed={setRenamedGatewayName}
            />
          )}
          renderRowSelection={
            showBulkControls
              ? (gateway) => (
                  <GatewaySelectionCheckbox
                    gateway={gateway}
                    isSelected={selectedIds.has(gateway.id)}
                    onToggle={toggleRowSelection}
                  />
                )
              : undefined
          }
          rows={visiblePage?.items ?? []}
          secondaryFilterControls={
            // "Show only my gateways" is a persistent on/off list filter, so it
            // is a PatternFly Switch (an on/off control) rather than a
            // pressed-state toggle button. The visible label is its accessible
            // name and the switch exposes a boolean on/off state. It sits on
            // its own toolbar row directly below the filtering controls,
            // following the console.redhat.com Clusters list layout
            // (WEB-UIEX-07).
            <Switch
              id="gateways-only-mine"
              isChecked={onlyMine}
              label={intl.formatMessage(messages.showOnlyMyGateways)}
              onChange={(_event, checked) => {
                changeOwnershipFilter(checked);
              }}
            />
          }
          selectionColumnLabel={intl.formatMessage(messages.selectionColumn)}
          state={tableState}
        />
        {bulkDelete.isConfirmOpen ? (
          <GatewayBulkDeleteConfirmModal
            isBusy={bulkDelete.isBusy}
            onCancel={bulkDelete.cancel}
            onConfirm={bulkDelete.confirm}
            targets={bulkDelete.targets}
          />
        ) : null}
        {connectionCommandGateway ? (
          <GatewayConnectionCommandModal
            gateway={connectionCommandGateway}
            isOpen={true}
            onClose={() => {
              setConnectionCommandGateway(null);
            }}
          />
        ) : null}
      </PageSection>
    </>
  );
}

export {
  GatewayPage,
  toGatewayDetailAttention,
  toGatewayDetailTab,
  type GatewayDetailAttention,
  type GatewayDetailTab,
  type GatewayPageProps,
} from "./gateway-detail-page";
