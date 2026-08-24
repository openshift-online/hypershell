import {
  Alert,
  Button,
  ClipboardCopy,
  Content,
  DescriptionList,
  DescriptionListDescription,
  DescriptionListGroup,
  DescriptionListTerm,
  ExpandableSection,
  Flex,
  FlexItem,
  Spinner,
  Stack,
  StackItem,
  Title,
} from "@patternfly/react-core";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { useIntl } from "react-intl";

import {
  defaultOpenShellGatewayServiceAccountListRequest,
  GatewayOperationError,
  openShellGatewayServiceAccountPageSizes,
  type OpenShellGatewayServiceAccountListRequest,
  type OpenShellGatewayServiceAccountRecord,
  type OpenShellGatewayServiceAccountSortField,
  type OpenShellGatewayServiceAccountStatus,
} from "../application/gateway-types";
import { useGatewayUi } from "../gateway-ui-provider";
import { GatewayStatus } from "../gateways/gateway-status";
import { messages } from "../messages";
import {
  ResourceTable,
  type ResourceTableColumn,
  type ResourceTableState,
  type ResourceTableStateChangeReason,
} from "../shared/resource-table";
import { ResourceRefreshButton } from "../shared/resource-refresh-button";
import { useDebouncedValue } from "../shared/use-debounced-value";
import { ServiceAccountCreateDialog } from "./service-account-create-dialog";
import {
  serviceAccountListQueryKey,
  serviceAccountNeedsPolling,
  serviceAccountSearchDebounceMilliseconds,
  serviceAccountStatusPollMilliseconds,
} from "./service-account-data";
import { ServiceAccountRowActions } from "./service-account-row-actions";

function isSortField(
  value: string,
): value is OpenShellGatewayServiceAccountSortField {
  return ["created_at", "expires_at", "name", "role", "status"].includes(value);
}

function serviceAccountStatusLabel(
  intl: ReturnType<typeof useIntl>,
  status: OpenShellGatewayServiceAccountStatus,
): string {
  const descriptor = {
    degraded: messages.serviceAccountStatusDegraded,
    deleting: messages.serviceAccountStatusDeleting,
    error: messages.serviceAccountStatusError,
    expired: messages.serviceAccountStatusExpired,
    provisioning: messages.serviceAccountStatusProvisioning,
    ready: messages.serviceAccountStatusReady,
    revoked: messages.serviceAccountStatusRevoked,
    revoking: messages.serviceAccountStatusRevoking,
  }[status];
  return intl.formatMessage(descriptor);
}

function LocalDate({ value }: { value: string }) {
  const intl = useIntl();
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? (
    intl.formatMessage(messages.notAvailable)
  ) : (
    <time dateTime={value}>
      {intl.formatDate(date, {
        dateStyle: "medium",
        timeStyle: "short",
      })}
    </time>
  );
}

function AccountDetails({
  account,
}: {
  account: OpenShellGatewayServiceAccountRecord;
}) {
  const intl = useIntl();
  const [isExpanded, setIsExpanded] = useState(false);
  return (
    <ExpandableSection
      isExpanded={isExpanded}
      onToggle={(_event, expanded) => {
        setIsExpanded(expanded);
      }}
      toggleText={intl.formatMessage(messages.viewServiceAccountDetails)}
    >
      <DescriptionList isCompact>
        <DescriptionListGroup>
          <DescriptionListTerm>
            {intl.formatMessage(messages.serviceAccountDescription)}
          </DescriptionListTerm>
          <DescriptionListDescription>
            {account.description ?? intl.formatMessage(messages.notAvailable)}
          </DescriptionListDescription>
        </DescriptionListGroup>
        <DescriptionListGroup>
          <DescriptionListTerm>
            {intl.formatMessage(messages.clientId)}
          </DescriptionListTerm>
          <DescriptionListDescription>
            {account.clientId}
          </DescriptionListDescription>
        </DescriptionListGroup>
        <DescriptionListGroup>
          <DescriptionListTerm>
            {intl.formatMessage(messages.subject)}
          </DescriptionListTerm>
          <DescriptionListDescription>
            <Stack hasGutter>
              <StackItem>
                <ClipboardCopy
                  clickTip={intl.formatMessage(messages.copied)}
                  copyAriaLabel={`${intl.formatMessage(messages.copy)} ${intl.formatMessage(messages.subject)}`}
                  hoverTip={intl.formatMessage(messages.copy)}
                  isCode
                  isReadOnly
                >
                  {account.subject}
                </ClipboardCopy>
              </StackItem>
              <StackItem>
                <Content component="p">
                  {intl.formatMessage(messages.subjectHelp)}
                </Content>
              </StackItem>
            </Stack>
          </DescriptionListDescription>
        </DescriptionListGroup>
        <DescriptionListGroup>
          <DescriptionListTerm>
            {intl.formatMessage(messages.created)}
          </DescriptionListTerm>
          <DescriptionListDescription>
            <LocalDate value={account.createdAt} />
          </DescriptionListDescription>
        </DescriptionListGroup>
      </DescriptionList>
    </ExpandableSection>
  );
}

export interface ServiceAccountsPageProps {
  collectionState?: OpenShellGatewayServiceAccountListRequest;
  gatewayId: string;
  isActive?: boolean;
  onCollectionStateChange?: (
    state: OpenShellGatewayServiceAccountListRequest,
    reason: ResourceTableStateChangeReason,
  ) => void;
}

export function ServiceAccountsPage({
  collectionState,
  gatewayId,
  isActive = true,
  onCollectionStateChange,
}: ServiceAccountsPageProps) {
  const intl = useIntl();
  const { gateways } = useGatewayUi();
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [localState, setLocalState] =
    useState<OpenShellGatewayServiceAccountListRequest>({
      ...defaultOpenShellGatewayServiceAccountListRequest,
    });
  const currentState = collectionState ?? localState;
  const debouncedSearch = useDebouncedValue(
    currentState.search.trim(),
    serviceAccountSearchDebounceMilliseconds,
  );
  const request = useMemo(
    () => ({ ...currentState, search: debouncedSearch }),
    [currentState, debouncedSearch],
  );
  const accounts = useQuery({
    enabled: isActive,
    placeholderData: keepPreviousData,
    queryFn: ({ signal }) =>
      gateways.listOpenShellGatewayServiceAccounts(gatewayId, request, signal),
    queryKey: serviceAccountListQueryKey(gatewayId, request),
    refetchInterval: ({ state }) =>
      isActive && state.data?.items.some(serviceAccountNeedsPolling)
        ? serviceAccountStatusPollMilliseconds
        : false,
    refetchIntervalInBackground: false,
    retry: (failureCount, error) =>
      failureCount < 1 &&
      (!(error instanceof GatewayOperationError) ||
        error.kind === "unavailable"),
    staleTime: 5_000,
  });
  const page = accounts.data;
  const capabilities = page?.capabilities;
  const tableState: ResourceTableState = {
    page: currentState.page,
    pageSize: currentState.size,
    query: currentState.search,
    sortColumnId: currentState.sort,
    sortDirection: currentState.order,
  };
  const changeState = (
    next: OpenShellGatewayServiceAccountListRequest,
    reason: ResourceTableStateChangeReason,
  ) => {
    if (onCollectionStateChange) {
      onCollectionStateChange(next, reason);
    } else {
      setLocalState(next);
    }
  };
  const changeTableState = (
    next: ResourceTableState,
    reason: ResourceTableStateChangeReason,
  ) => {
    changeState(
      {
        ...currentState,
        order: next.sortDirection,
        page: next.page,
        search: next.query,
        size: next.pageSize,
        sort: isSortField(next.sortColumnId) ? next.sortColumnId : "created_at",
      },
      reason,
    );
  };
  const columns: readonly ResourceTableColumn<OpenShellGatewayServiceAccountRecord>[] =
    [
      {
        id: "name",
        label: intl.formatMessage(messages.serviceAccountName),
        render: (account) => (
          <Stack>
            <StackItem>{account.name}</StackItem>
            <StackItem>
              <AccountDetails account={account} />
            </StackItem>
          </Stack>
        ),
        width: 35,
      },
      {
        id: "role",
        label: intl.formatMessage(messages.serviceAccountRole),
        render: ({ role }) => role,
        width: 20,
      },
      {
        id: "status",
        label: intl.formatMessage(messages.status),
        render: ({ status }) => (
          <GatewayStatus
            label={serviceAccountStatusLabel(intl, status)}
            status={status}
          />
        ),
        width: 20,
      },
      {
        id: "expires_at",
        label: intl.formatMessage(messages.expiration),
        render: ({ expiresAt }) => <LocalDate value={expiresAt} />,
        width: 25,
      },
    ];

  return (
    <Stack hasGutter>
      <StackItem>
        <Flex
          alignItems={{ default: "alignItemsFlexStart" }}
          justifyContent={{ default: "justifyContentSpaceBetween" }}
        >
          <FlexItem>
            <Title headingLevel="h2" size="xl">
              {intl.formatMessage(messages.serviceAccountsHeading)}
            </Title>
            <Content component="p">
              {intl.formatMessage(messages.serviceAccountsDescription)}
            </Content>
          </FlexItem>
          <FlexItem>
            <ResourceRefreshButton
              ariaLabel={intl.formatMessage(messages.refreshServiceAccounts)}
              isRefreshing={accounts.isFetching}
              onRefresh={() => accounts.refetch()}
            />
          </FlexItem>
        </Flex>
      </StackItem>
      {capabilities && !capabilities.canManageAll ? (
        <StackItem>
          <Alert
            isInline
            title={intl.formatMessage(messages.serviceAccountOwnScope)}
            variant="info"
          />
        </StackItem>
      ) : null}
      {capabilities && !capabilities.canCreate ? (
        <StackItem>
          <Alert
            id="service-account-create-denied"
            isInline
            title={intl.formatMessage(messages.serviceAccountCreateDenied)}
            variant="info"
          />
        </StackItem>
      ) : null}
      {accounts.isError ? (
        <StackItem>
          <Alert
            actionLinks={
              <Button
                isInline
                onClick={() => void accounts.refetch()}
                variant="link"
              >
                {intl.formatMessage(messages.retry)}
              </Button>
            }
            isInline
            title={intl.formatMessage(messages.serviceAccountsLoadError)}
            variant="danger"
          >
            {intl.formatMessage(messages.serviceAccountsLoadErrorBody)}
          </Alert>
        </StackItem>
      ) : null}
      {!page && accounts.isPending ? (
        <StackItem>
          <Spinner
            aria-label={intl.formatMessage(messages.loadingServiceAccount)}
          />
        </StackItem>
      ) : null}
      {page ? (
        <StackItem>
          <ResourceTable
            ariaLabel={intl.formatMessage(messages.serviceAccountsHeading)}
            columns={columns}
            getRowKey={({ id }) => id}
            hasActiveFilters={
              currentState.search.trim().length > 0 ||
              currentState.status !== undefined
            }
            id="gateway-service-accounts"
            itemCount={page.total}
            labels={{
              actions: intl.formatMessage(messages.actions),
              clearFilters: intl.formatMessage(messages.clearFilters),
              emptyBody: intl.formatMessage(messages.serviceAccountsEmptyBody),
              emptyTitle: intl.formatMessage(
                messages.serviceAccountsEmptyTitle,
              ),
              noResultsBody: intl.formatMessage(
                messages.noMatchingServiceAccountsBody,
              ),
              noResultsTitle: intl.formatMessage(
                messages.noMatchingServiceAccounts,
              ),
              resultsCountContext: intl.formatMessage(messages.results),
              searchAriaLabel: intl.formatMessage(
                messages.filterServiceAccounts,
              ),
              searchPlaceholder: intl.formatMessage(
                messages.filterServiceAccounts,
              ),
            }}
            onClearFilters={() => {
              changeState(
                {
                  ...currentState,
                  page: 1,
                  search: "",
                  status: undefined,
                },
                "filter",
              );
            }}
            onStateChange={changeTableState}
            pageSizeOptions={openShellGatewayServiceAccountPageSizes}
            primaryAction={
              <Button
                aria-describedby={
                  page.capabilities.canCreate
                    ? undefined
                    : "service-account-create-denied"
                }
                isDisabled={!page.capabilities.canCreate}
                onClick={() => {
                  setIsCreateOpen(true);
                }}
                variant="primary"
              >
                {intl.formatMessage(messages.createServiceAccount)}
              </Button>
            }
            renderRowAction={(account) => (
              <ServiceAccountRowActions
                account={account}
                gatewayId={gatewayId}
              />
            )}
            rows={page.items}
            state={tableState}
          />
        </StackItem>
      ) : null}
      {isCreateOpen && capabilities ? (
        <ServiceAccountCreateDialog
          capabilities={capabilities}
          gatewayId={gatewayId}
          isOpen
          onClose={() => {
            setIsCreateOpen(false);
          }}
        />
      ) : null}
    </Stack>
  );
}
