import {
  Alert,
  Button,
  Content,
  Flex,
  FlexItem,
  Label,
  MenuToggle,
  Select,
  SelectList,
  SelectOption,
  Spinner,
  Split,
  SplitItem,
  Stack,
  StackItem,
  Title,
  type MenuToggleElement,
} from "@patternfly/react-core";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { useIntl } from "react-intl";

import {
  defaultGatewayAccessListRequest,
  gatewayAccessPageSizes,
  GatewayOperationError,
  type GatewayAccessGrantRecord,
  type GatewayAccessListRequest,
  type GatewayAccessRole,
} from "../application/gateway-types";
import { useGatewayUi } from "../gateway-ui-provider";
import { messages } from "../messages";
import { ResourceRefreshButton } from "../shared/resource-refresh-button";
import {
  ResourceTable,
  type ResourceTableColumn,
  type ResourceTableState,
  type ResourceTableStateChangeReason,
} from "../shared/resource-table";
import { useDebouncedValue } from "../shared/use-debounced-value";
import {
  accessListQueryKey,
  accessSearchDebounceMilliseconds,
} from "./access-data";
import { AccessRemoveAction, AccessRoleControl } from "./access-row-actions";
import { roleLabel, rowDisabledReason } from "./access-role";
import { AddUsersDialog } from "./add-users-dialog";

function RoleFilter({
  onChange,
  value,
}: {
  onChange: (role: GatewayAccessRole | undefined) => void;
  value: GatewayAccessRole | undefined;
}) {
  const intl = useIntl();
  const [isOpen, setIsOpen] = useState(false);
  const label = value
    ? roleLabel(intl.formatMessage, value)
    : intl.formatMessage(messages.accessRoleFilterAll);
  const toggle = (toggleRef: React.Ref<MenuToggleElement>) => (
    <MenuToggle
      aria-label={intl.formatMessage(messages.accessRoleFilterLabel)}
      isExpanded={isOpen}
      onClick={() => {
        setIsOpen((open) => !open);
      }}
      ref={toggleRef}
    >
      {label}
    </MenuToggle>
  );
  return (
    <Select
      id="gateway-access-role-filter"
      isOpen={isOpen}
      onOpenChange={setIsOpen}
      onSelect={(_event, selected) => {
        setIsOpen(false);
        onChange(
          selected === "all" ? undefined : (selected as GatewayAccessRole),
        );
      }}
      selected={value ?? "all"}
      toggle={toggle}
    >
      <SelectList>
        <SelectOption value="all">
          {intl.formatMessage(messages.accessRoleFilterAll)}
        </SelectOption>
        <SelectOption value="owner">
          {intl.formatMessage(messages.accessRoleOwner)}
        </SelectOption>
        <SelectOption value="admin">
          {intl.formatMessage(messages.accessRoleAdmin)}
        </SelectOption>
        <SelectOption value="user">
          {intl.formatMessage(messages.accessRoleUser)}
        </SelectOption>
      </SelectList>
    </Select>
  );
}

export interface AccessPageProps {
  collectionState?: GatewayAccessListRequest;
  gatewayId: string;
  isActive?: boolean;
  onCollectionStateChange?: (
    state: GatewayAccessListRequest,
    reason: ResourceTableStateChangeReason,
  ) => void;
}

export function AccessPage({
  collectionState,
  gatewayId,
  isActive = true,
  onCollectionStateChange,
}: AccessPageProps) {
  const intl = useIntl();
  const { gateways } = useGatewayUi();
  const [isAddOpen, setIsAddOpen] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);
  const [localState, setLocalState] = useState<GatewayAccessListRequest>({
    ...defaultGatewayAccessListRequest,
  });
  const currentState = collectionState ?? localState;
  const debouncedSearch = useDebouncedValue(
    currentState.search.trim(),
    accessSearchDebounceMilliseconds,
  );
  const request = useMemo(
    () => ({ ...currentState, search: debouncedSearch }),
    [currentState, debouncedSearch],
  );
  const access = useQuery({
    enabled: isActive,
    placeholderData: keepPreviousData,
    queryFn: ({ signal }) =>
      gateways.listGatewayAccess(gatewayId, request, signal),
    queryKey: accessListQueryKey(gatewayId, request),
    retry: (failureCount, error) =>
      failureCount < 1 &&
      (!(error instanceof GatewayOperationError) ||
        error.kind === "unavailable"),
    staleTime: 5_000,
  });
  const page = access.data;
  const capabilities = page?.capabilities;
  const canManage = capabilities?.canManageAccess ?? false;
  const ownerCount = useMemo(
    () => (page?.items ?? []).filter((item) => item.role === "owner").length,
    [page],
  );

  const changeState = (
    next: GatewayAccessListRequest,
    reason: ResourceTableStateChangeReason,
  ) => {
    if (onCollectionStateChange) {
      onCollectionStateChange(next, reason);
    } else {
      setLocalState(next);
    }
  };
  const tableState: ResourceTableState = {
    page: currentState.page,
    pageSize: currentState.size,
    query: currentState.search,
    sortColumnId: currentState.sort,
    sortDirection: currentState.order,
  };
  const changeTableState = (
    next: ResourceTableState,
    reason: ResourceTableStateChangeReason,
  ) => {
    changeState(
      {
        ...currentState,
        page: next.page,
        search: next.query,
        size: next.pageSize,
      },
      reason,
    );
  };

  const columns: readonly ResourceTableColumn<GatewayAccessGrantRecord>[] = [
    {
      id: "name",
      label: intl.formatMessage(messages.accessUserName),
      render: (grant) => (
        <Split hasGutter>
          <SplitItem>
            {grant.name?.trim() ? grant.name : grant.username}
          </SplitItem>
          {grant.isCreator ? (
            <SplitItem>
              <Label isCompact>
                {intl.formatMessage(messages.accessCreatorMarker)}
              </Label>
            </SplitItem>
          ) : null}
        </Split>
      ),
      sortable: false,
      width: 35,
    },
    {
      id: "username",
      label: intl.formatMessage(messages.accessUserId),
      render: (grant) => grant.username,
      sortable: false,
      width: 35,
    },
    {
      id: "role",
      label: intl.formatMessage(messages.accessRoleColumn),
      render: (grant) =>
        canManage && capabilities ? (
          <AccessRoleControl
            capabilities={capabilities}
            disabledReason={rowDisabledReason(
              intl.formatMessage,
              grant,
              capabilities,
              ownerCount,
            )}
            gatewayId={gatewayId}
            grant={grant}
            onActionError={setActionError}
          />
        ) : (
          roleLabel(intl.formatMessage, grant.role)
        ),
      sortable: false,
      width: 30,
    },
  ];

  const hasActiveFilters =
    currentState.search.trim().length > 0 || currentState.role !== undefined;

  return (
    <Stack hasGutter>
      <StackItem>
        <Flex
          alignItems={{ default: "alignItemsFlexStart" }}
          justifyContent={{ default: "justifyContentSpaceBetween" }}
        >
          <FlexItem>
            <Title headingLevel="h2" size="xl">
              {intl.formatMessage(messages.accessHeading)}
            </Title>
            <Content component="p">
              {intl.formatMessage(messages.accessDescription)}
            </Content>
          </FlexItem>
          <FlexItem>
            <ResourceRefreshButton
              ariaLabel={intl.formatMessage(messages.accessRefresh)}
              isRefreshing={access.isFetching}
              onRefresh={() => access.refetch()}
            />
          </FlexItem>
        </Flex>
      </StackItem>
      {capabilities && !canManage ? (
        <StackItem>
          <Alert
            isInline
            title={intl.formatMessage(messages.accessViewerReadOnly)}
            variant="info"
          />
        </StackItem>
      ) : null}
      {actionError ? (
        <StackItem>
          <Alert
            actionClose={
              <Button
                onClick={() => {
                  setActionError(null);
                }}
                variant="plain"
              >
                {intl.formatMessage(messages.cancel)}
              </Button>
            }
            isInline
            title={actionError}
            variant="danger"
          />
        </StackItem>
      ) : null}
      {access.isError ? (
        <StackItem>
          <Alert
            actionLinks={
              <Button
                isInline
                onClick={() => void access.refetch()}
                variant="link"
              >
                {intl.formatMessage(messages.retry)}
              </Button>
            }
            isInline
            title={intl.formatMessage(messages.accessLoadError)}
            variant="danger"
          >
            {intl.formatMessage(messages.accessLoadErrorBody)}
          </Alert>
        </StackItem>
      ) : null}
      {!page && access.isPending ? (
        <StackItem>
          <Spinner aria-label={intl.formatMessage(messages.accessLoading)} />
        </StackItem>
      ) : null}
      {page ? (
        <StackItem>
          <ResourceTable
            ariaLabel={intl.formatMessage(messages.accessHeading)}
            columns={columns}
            filterControls={
              <RoleFilter
                onChange={(role) => {
                  changeState({ ...currentState, page: 1, role }, "filter");
                }}
                value={currentState.role}
              />
            }
            getRowKey={(grant) => grant.userId}
            hasActiveFilters={hasActiveFilters}
            id="gateway-access"
            itemCount={page.total}
            labels={{
              actions: intl.formatMessage(messages.actions),
              clearFilters: intl.formatMessage(messages.clearFilters),
              emptyBody: intl.formatMessage(messages.accessEmptyBody),
              emptyTitle: intl.formatMessage(messages.accessEmptyTitle),
              noResultsBody: intl.formatMessage(messages.accessNoResultsBody),
              noResultsTitle: intl.formatMessage(messages.accessNoResultsTitle),
              resultsCountContext: intl.formatMessage(messages.results),
              searchAriaLabel: intl.formatMessage(messages.accessFindPeople),
              searchPlaceholder: intl.formatMessage(messages.accessFindPeople),
            }}
            onClearFilters={() => {
              changeState(
                { ...currentState, page: 1, role: undefined, search: "" },
                "filter",
              );
            }}
            onStateChange={changeTableState}
            pageSizeOptions={gatewayAccessPageSizes}
            primaryAction={
              canManage ? (
                <Button
                  onClick={() => {
                    setIsAddOpen(true);
                  }}
                  variant="primary"
                >
                  {intl.formatMessage(messages.accessAddUsers)}
                </Button>
              ) : undefined
            }
            renderRowAction={
              canManage && capabilities
                ? (grant) => (
                    <AccessRemoveAction
                      disabledReason={rowDisabledReason(
                        intl.formatMessage,
                        grant,
                        capabilities,
                        ownerCount,
                      )}
                      gatewayId={gatewayId}
                      grant={grant}
                    />
                  )
                : undefined
            }
            rows={page.items}
            state={tableState}
          />
        </StackItem>
      ) : null}
      {isAddOpen && capabilities ? (
        <AddUsersDialog
          capabilities={capabilities}
          gatewayId={gatewayId}
          isOpen
          onClose={() => {
            setIsAddOpen(false);
          }}
        />
      ) : null}
    </Stack>
  );
}
