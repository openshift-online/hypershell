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
  PageSection,
  Title,
} from "@patternfly/react-core";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { FormattedMessage, useIntl } from "react-intl";

import {
  defaultGatewayProfileListRequest,
  gatewayProfileListPageSizes,
  type GatewayProfileListRequest,
  type GatewayProfileRecord,
  type GatewayProfileSortField,
} from "../application/gateway-profile-types";
import {
  useGatewayProfileLink,
  useGatewayProfileUi,
} from "../gateway-profile-ui-provider";
import { gatewayProfileMessages } from "../gateway-profile-messages";
import { messages } from "../messages";
import {
  gatewayProfileListQueryKey,
  gatewayProfileQueryKey,
  gatewayProfileSearchDebounceMilliseconds,
} from "../gateway-profiles/gateway-profile-data";
import { GatewayProfileDeleteDialog } from "../gateway-profiles/gateway-profile-delete-dialog";
import { GatewayProfileLoadState } from "../gateway-profiles/gateway-profile-load-state";
import { GatewayProfileRowActions } from "../gateway-profiles/gateway-profile-row-actions";
import {
  ResourceTable,
  type ResourceTableColumn,
  type ResourceTableState,
  type ResourceTableStateChangeReason,
} from "../shared/resource-table";
import { ResourceRefreshButton } from "../shared/resource-refresh-button";
import { useDebouncedValue } from "../shared/use-debounced-value";

const primaryLinkStyle: React.CSSProperties = {
  color: "var(--pf-v6-c-button--m-primary--Color)",
  textDecoration: "none",
};

export interface GatewayProfilesPageProps {
  collectionState?: GatewayProfileListRequest;
  deletedGatewayProfileName?: string;
  gatewayProfiles?: readonly GatewayProfileRecord[];
  onCollectionStateChange?: (
    state: GatewayProfileListRequest,
    reason: ResourceTableStateChangeReason,
  ) => void;
  onDismissDeletedGatewayProfile?: () => void;
  onRefresh?: () => unknown;
}

function isGatewayProfileSortField(
  value: string,
): value is GatewayProfileSortField {
  return ["created", "name"].includes(value);
}

function GatewayProfileDetailLink({
  gatewayProfile,
}: {
  gatewayProfile: GatewayProfileRecord;
}) {
  const { navigation } = useGatewayProfileUi();
  const link = useGatewayProfileLink(navigation.detailHref(gatewayProfile.id));

  return <a {...link}>{gatewayProfile.name}</a>;
}

function GatewayProfileCreatedDate({ createdAt }: { createdAt?: string }) {
  const intl = useIntl();
  const createdDate = createdAt ? new Date(createdAt) : undefined;

  if (!createdDate || Number.isNaN(createdDate.getTime())) {
    return intl.formatMessage(messages.notAvailable);
  }

  return (
    <time dateTime={createdAt}>
      {intl.formatDate(createdDate, {
        day: "numeric",
        month: "short",
        year: "numeric",
      })}
    </time>
  );
}

function GatewayProfileDeletedAlert({
  deletedGatewayProfileName,
  onDismissDeleted,
}: {
  deletedGatewayProfileName?: string;
  onDismissDeleted: () => void;
}) {
  const intl = useIntl();

  if (!deletedGatewayProfileName) {
    return null;
  }

  return (
    <AlertGroup
      aria-label={intl.formatMessage(messages.notifications)}
      isLiveRegion
      isToast
    >
      <Alert
        actionClose={
          <AlertActionCloseButton
            aria-label={intl.formatMessage(messages.close)}
            onClose={onDismissDeleted}
          />
        }
        onTimeout={onDismissDeleted}
        timeout={8000}
        title={intl.formatMessage(
          gatewayProfileMessages.gatewayProfileDeleted,
          {
            gatewayProfileName: deletedGatewayProfileName,
          },
        )}
        variant="success"
      />
    </AlertGroup>
  );
}

export function GatewayProfilesPage({
  collectionState,
  deletedGatewayProfileName: initialDeletedGatewayProfileName,
  gatewayProfiles,
  onCollectionStateChange,
  onDismissDeletedGatewayProfile,
  onRefresh,
}: GatewayProfilesPageProps = {}) {
  const intl = useIntl();
  const { gatewayProfiles: gatewayProfileOperations, navigation } =
    useGatewayProfileUi();
  const createLink = useGatewayProfileLink(navigation.createHref);
  const [deletedGatewayProfileName, setDeletedGatewayProfileName] =
    useState<string>();
  const [isInitialDeletionDismissed, setIsInitialDeletionDismissed] =
    useState(false);
  const [localCollectionState, setLocalCollectionState] =
    useState<GatewayProfileListRequest>({
      ...defaultGatewayProfileListRequest,
    });
  const currentCollectionState = collectionState ?? localCollectionState;
  const debouncedSearch = useDebouncedValue(
    currentCollectionState.search.trim(),
    gatewayProfileSearchDebounceMilliseconds,
  );
  const request: GatewayProfileListRequest = {
    ...currentCollectionState,
    search: debouncedSearch,
  };
  const gatewayProfileQuery = useQuery({
    enabled: gatewayProfiles === undefined,
    placeholderData: keepPreviousData,
    queryFn: ({ signal }) =>
      gatewayProfileOperations.listGatewayProfiles(request, signal),
    queryKey: gatewayProfileListQueryKey(request),
  });
  const visiblePage = gatewayProfiles
    ? {
        items: gatewayProfiles,
        page: currentCollectionState.page,
        size: currentCollectionState.size,
        total: gatewayProfiles.length,
      }
    : gatewayProfileQuery.data;
  const tableState: ResourceTableState = {
    page: currentCollectionState.page,
    pageSize: currentCollectionState.size,
    query: currentCollectionState.search,
    sortColumnId: currentCollectionState.sortField,
    sortDirection: currentCollectionState.sortDirection,
  };
  const changeTableState = (
    nextState: ResourceTableState,
    reason: ResourceTableStateChangeReason,
  ) => {
    const nextCollectionState: GatewayProfileListRequest = {
      page: nextState.page,
      search: nextState.query,
      size: nextState.pageSize,
      sortDirection: nextState.sortDirection,
      sortField: isGatewayProfileSortField(nextState.sortColumnId)
        ? nextState.sortColumnId
        : "name",
    };
    if (onCollectionStateChange) {
      onCollectionStateChange(nextCollectionState, reason);
    } else {
      setLocalCollectionState(nextCollectionState);
    }
  };
  const columns: readonly ResourceTableColumn<GatewayProfileRecord>[] = [
    {
      id: "name",
      label: intl.formatMessage(gatewayProfileMessages.gatewayProfileName),
      render: (gatewayProfile) => (
        <GatewayProfileDetailLink gatewayProfile={gatewayProfile} />
      ),
      width: 25,
    },
    {
      id: "description",
      label: intl.formatMessage(
        gatewayProfileMessages.gatewayProfileDescription,
      ),
      render: ({ description }) =>
        description ?? intl.formatMessage(messages.notAvailable),
      sortable: false,
    },
    {
      id: "podCount",
      label: intl.formatMessage(gatewayProfileMessages.podCount),
      render: ({ podCount }) =>
        typeof podCount === "number"
          ? String(podCount)
          : intl.formatMessage(messages.notAvailable),
      sortable: false,
      width: 15,
    },
    {
      id: "created",
      label: intl.formatMessage(messages.created),
      render: ({ createdAt }) => (
        <GatewayProfileCreatedDate createdAt={createdAt} />
      ),
      width: 15,
    },
  ];

  if (!visiblePage && gatewayProfileQuery.isPending) {
    return <GatewayProfileLoadState />;
  }
  if (!visiblePage && gatewayProfileQuery.isError) {
    return <GatewayProfileLoadState isError />;
  }

  return (
    <>
      <GatewayProfileDeletedAlert
        deletedGatewayProfileName={
          deletedGatewayProfileName ??
          (isInitialDeletionDismissed
            ? undefined
            : initialDeletedGatewayProfileName)
        }
        onDismissDeleted={() => {
          setDeletedGatewayProfileName(undefined);
          setIsInitialDeletionDismissed(true);
          onDismissDeletedGatewayProfile?.();
        }}
      />
      <PageSection hasBodyWrapper={false}>
        <Flex
          alignItems={{ default: "alignItemsFlexStart" }}
          justifyContent={{ default: "justifyContentSpaceBetween" }}
        >
          <FlexItem>
            <Title headingLevel="h1" size="2xl">
              <FormattedMessage {...gatewayProfileMessages.gatewayProfiles} />
            </Title>
          </FlexItem>
          {gatewayProfiles === undefined || onRefresh ? (
            <FlexItem>
              <ResourceRefreshButton
                ariaLabel={intl.formatMessage(
                  gatewayProfileMessages.refreshGatewayProfiles,
                )}
                isRefreshing={gatewayProfileQuery.isFetching}
                onRefresh={onRefresh ?? (() => gatewayProfileQuery.refetch())}
              />
            </FlexItem>
          ) : null}
        </Flex>
      </PageSection>
      <PageSection hasBodyWrapper={false} isFilled>
        <ResourceTable
          ariaLabel={intl.formatMessage(gatewayProfileMessages.gatewayProfiles)}
          columns={columns}
          getRowKey={({ id }) => id}
          id="gateway-profiles"
          itemCount={visiblePage?.total ?? 0}
          labels={{
            actions: intl.formatMessage(messages.actions),
            clearFilters: intl.formatMessage(messages.clearFilters),
            emptyBody: intl.formatMessage(
              gatewayProfileMessages.gatewayProfilesEmptyBody,
            ),
            emptyTitle: intl.formatMessage(
              gatewayProfileMessages.gatewayProfilesEmptyTitle,
            ),
            noResultsBody: intl.formatMessage(
              gatewayProfileMessages.noMatchingGatewayProfilesBody,
            ),
            noResultsTitle: intl.formatMessage(
              gatewayProfileMessages.noMatchingGatewayProfiles,
            ),
            resultsCountContext: intl.formatMessage(messages.results),
            searchAriaLabel: intl.formatMessage(
              gatewayProfileMessages.filterGatewayProfiles,
            ),
            searchPlaceholder: intl.formatMessage(
              gatewayProfileMessages.filterGatewayProfiles,
            ),
          }}
          primaryAction={
            <Button
              component="a"
              style={primaryLinkStyle}
              variant="primary"
              {...createLink}
            >
              <FormattedMessage
                {...gatewayProfileMessages.createGatewayProfile}
              />
            </Button>
          }
          onStateChange={changeTableState}
          pageSizeOptions={gatewayProfileListPageSizes}
          renderRowAction={(gatewayProfile) => (
            <GatewayProfileRowActions
              gatewayProfile={gatewayProfile}
              onDeleted={() => {
                setDeletedGatewayProfileName(gatewayProfile.name);
              }}
            />
          )}
          rows={visiblePage?.items ?? []}
          state={tableState}
        />
      </PageSection>
    </>
  );
}

export interface GatewayProfilePageProps {
  gatewayProfile?: GatewayProfileRecord;
  gatewayProfileId: string;
  onDeleted?: (gatewayProfileName: string) => Promise<void> | void;
}

export function GatewayProfilePage({
  gatewayProfile,
  gatewayProfileId,
  onDeleted,
}: GatewayProfilePageProps) {
  const intl = useIntl();
  const { gatewayProfiles, navigation } = useGatewayProfileUi();
  const [isDeleteOpen, setIsDeleteOpen] = useState(false);
  const gatewayProfileQuery = useQuery({
    enabled: gatewayProfile === undefined && gatewayProfileId.length > 0,
    queryFn: ({ signal }) =>
      gatewayProfiles.getGatewayProfile(gatewayProfileId, signal),
    queryKey: gatewayProfileQueryKey(gatewayProfileId),
  });
  const visibleGatewayProfile = gatewayProfile ?? gatewayProfileQuery.data;

  if (!visibleGatewayProfile && gatewayProfileQuery.isPending) {
    return <GatewayProfileLoadState />;
  }
  if (!visibleGatewayProfile) {
    return <GatewayProfileLoadState isError />;
  }

  const quotaFields: readonly {
    label: string;
    value?: number | string;
  }[] = [
    {
      label: intl.formatMessage(gatewayProfileMessages.cpuRequestTotal),
      value: visibleGatewayProfile.cpuRequestTotal,
    },
    {
      label: intl.formatMessage(gatewayProfileMessages.cpuLimitTotal),
      value: visibleGatewayProfile.cpuLimitTotal,
    },
    {
      label: intl.formatMessage(gatewayProfileMessages.memoryRequestTotal),
      value: visibleGatewayProfile.memoryRequestTotal,
    },
    {
      label: intl.formatMessage(gatewayProfileMessages.memoryLimitTotal),
      value: visibleGatewayProfile.memoryLimitTotal,
    },
    {
      label: intl.formatMessage(gatewayProfileMessages.ephemeralStorageTotal),
      value: visibleGatewayProfile.ephemeralStorageTotal,
    },
    {
      label: intl.formatMessage(
        gatewayProfileMessages.containerCpuRequestDefault,
      ),
      value: visibleGatewayProfile.containerCpuRequestDefault,
    },
    {
      label: intl.formatMessage(gatewayProfileMessages.containerCpuLimitMax),
      value: visibleGatewayProfile.containerCpuLimitMax,
    },
    {
      label: intl.formatMessage(
        gatewayProfileMessages.containerMemoryRequestDefault,
      ),
      value: visibleGatewayProfile.containerMemoryRequestDefault,
    },
    {
      label: intl.formatMessage(gatewayProfileMessages.containerMemoryLimitMax),
      value: visibleGatewayProfile.containerMemoryLimitMax,
    },
    {
      label: intl.formatMessage(gatewayProfileMessages.podCount),
      value: visibleGatewayProfile.podCount,
    },
    {
      label: intl.formatMessage(gatewayProfileMessages.pvcCount),
      value: visibleGatewayProfile.pvcCount,
    },
  ];

  return (
    <>
      <PageSection hasBodyWrapper={false}>
        <Flex
          alignItems={{ default: "alignItemsFlexStart" }}
          justifyContent={{ default: "justifyContentSpaceBetween" }}
        >
          <FlexItem>
            <Title headingLevel="h1" size="2xl">
              {visibleGatewayProfile.name}
            </Title>
          </FlexItem>
          <FlexItem>
            <Button
              onClick={() => {
                setIsDeleteOpen(true);
              }}
              variant="secondary"
            >
              <FormattedMessage
                {...gatewayProfileMessages.deleteGatewayProfile}
              />
            </Button>
          </FlexItem>
        </Flex>
      </PageSection>
      <PageSection hasBodyWrapper={false} isFilled variant="secondary">
        <DescriptionList isHorizontal>
          <DescriptionListGroup>
            <DescriptionListTerm>
              <FormattedMessage
                {...gatewayProfileMessages.gatewayProfileDescription}
              />
            </DescriptionListTerm>
            <DescriptionListDescription>
              {visibleGatewayProfile.description ?? (
                <FormattedMessage {...messages.notAvailable} />
              )}
            </DescriptionListDescription>
          </DescriptionListGroup>
          {quotaFields.map(({ label, value }) => (
            <DescriptionListGroup key={label}>
              <DescriptionListTerm>{label}</DescriptionListTerm>
              <DescriptionListDescription>
                {value === undefined ? (
                  <FormattedMessage {...messages.notAvailable} />
                ) : (
                  String(value)
                )}
              </DescriptionListDescription>
            </DescriptionListGroup>
          ))}
        </DescriptionList>
      </PageSection>
      <GatewayProfileDeleteDialog
        gatewayProfileId={visibleGatewayProfile.id}
        gatewayProfileName={visibleGatewayProfile.name}
        isOpen={isDeleteOpen}
        onClose={() => {
          setIsDeleteOpen(false);
        }}
        onDeleted={() => {
          setIsDeleteOpen(false);
          if (onDeleted) {
            void onDeleted(visibleGatewayProfile.name);
          } else {
            void navigation.navigate(navigation.collectionHref, {
              state: {
                deletedGatewayProfileName: visibleGatewayProfile.name,
              },
            });
          }
        }}
      />
    </>
  );
}
