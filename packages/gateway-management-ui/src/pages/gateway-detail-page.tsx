import {
  Alert,
  AlertActionCloseButton,
  Button,
  Card,
  CardBody,
  CardHeader,
  CardTitle,
  Content,
  DescriptionList,
  DescriptionListDescription,
  DescriptionListGroup,
  DescriptionListTerm,
  Flex,
  FlexItem,
  Grid,
  GridItem,
  PageSection,
  Stack,
  StackItem,
  Tab,
  Tabs,
  TabTitleText,
  Title,
} from "@patternfly/react-core";
import { ExternalLinkAltIcon, SyncAltIcon } from "@patternfly/react-icons";
import { useQuery } from "@tanstack/react-query";
import { useCallback, useRef, useState } from "react";
import { FormattedMessage, useIntl } from "react-intl";
import {
  defaultOpenShellGatewayServiceAccountListRequest,
  type GatewayRecord,
  type OpenShellGatewayServiceAccountListRequest,
} from "../application/gateway-types";
import { useGatewayUi } from "../gateway-ui-provider";
import { useConsoleWaitTracker } from "../gateways/console-wait-tracker";
import {
  type GatewayConnection,
  gatewayStatusAppearance,
} from "../gateways/gateway-connections";
import { GatewayConnectionSteps } from "../gateways/gateway-connection-steps";
import {
  GatewayDetailHeader,
  GatewayEndpointCopy,
} from "../gateways/gateway-detail-header";
import { GatewayProvisioningStepper } from "../gateways/gateway-provisioning-stepper";
import {
  deletedGatewayNavigation,
  gatewayNeedsStatusPolling,
  gatewayPlacementDetailQueryKey,
  gatewayPlacementStaleMilliseconds,
  gatewayQueryKey,
  gatewayStatusPollMilliseconds,
  toGatewayConnection,
} from "../gateways/gateway-data";
import { GatewayLoadState } from "../gateways/gateway-load-state";
import { GatewayStatus } from "../gateways/gateway-status";
import { AccessPage } from "../access/access-page";
import { type ResourceTableStateChangeReason } from "../shared/resource-table";
import { ServiceAccountsPage } from "../service-accounts/service-accounts-page";
import { type ServiceAccountLeaveGuard } from "../service-accounts/service-account-create-dialog";
import {
  isServiceAccountExpiringSoon,
  serviceAccountListQueryKey,
} from "../service-accounts/service-account-data";
import { messages } from "../messages";
import {
  GatewayPoliciesTab,
  sandboxResourcesFromDefaults,
} from "../policies/gateway-policies-tab";
import { gatewayPoliciesMessages } from "../policies/gateway-policies-messages";
import {
  gatewayEffectivePoliciesQueryKey,
  gatewayEffectivePoliciesStaleMilliseconds,
} from "../policies/policy-data";
import { GatewayCreatedDate, GatewaySuccessAlerts } from "./gateway-page-parts";
import styles from "./gateway-pages.module.css";

function GatewayDetailClusterName({ gateway }: { gateway: GatewayConnection }) {
  const intl = useIntl();
  const { gateways } = useGatewayUi();
  const clusterId = gateway.clusterId;
  const placementQuery = useQuery({
    queryFn: ({ signal }) => gateways.getGatewayPlacement(clusterId, signal),
    queryKey: gatewayPlacementDetailQueryKey(clusterId),
    staleTime: gatewayPlacementStaleMilliseconds,
  });

  if (placementQuery.isPending) {
    return (
      <span role="status">
        {intl.formatMessage(messages.loadingClusterName)}
      </span>
    );
  }
  if (placementQuery.isError) {
    return (
      <>
        {intl.formatMessage(messages.notAvailable)}{" "}
        <Button
          isInline
          onClick={() => void placementQuery.refetch()}
          variant="link"
        >
          {intl.formatMessage(messages.retry)}
        </Button>
      </>
    );
  }
  return placementQuery.data.name;
}

export type GatewayDetailTab =
  "connection" | "details" | "policies" | "user-access" | "service-accounts";

export type GatewayDetailAttention =
  | "gateway-health"
  | "sandbox-expiring"
  | "sandbox-idle"
  | "service-account-expiring";

const gatewayDetailAttentions: readonly GatewayDetailAttention[] = [
  "gateway-health",
  "sandbox-expiring",
  "sandbox-idle",
  "service-account-expiring",
];

/** Normalizes an attention value so unknown values do not create UI states. */
export function toGatewayDetailAttention(
  value: string | null | undefined,
): GatewayDetailAttention | undefined {
  return value && (gatewayDetailAttentions as readonly string[]).includes(value)
    ? (value as GatewayDetailAttention)
    : undefined;
}

const gatewayDetailTabs: readonly GatewayDetailTab[] = [
  "details",
  "connection",
  "user-access",
  "service-accounts",
  "policies",
];

/** Normalizes an arbitrary tab value, defaulting unknown values to Details. */
export function toGatewayDetailTab(
  value: string | null | undefined,
): GatewayDetailTab {
  return value && (gatewayDetailTabs as readonly string[]).includes(value)
    ? (value as GatewayDetailTab)
    : "details";
}

export interface GatewayPageProps {
  activeTab?: GatewayDetailTab;
  attention?: GatewayDetailAttention;
  gateway?: GatewayRecord;
  gatewayId: string;
  /** Policy whose details drawer is open when the Policies tab first renders. */
  initialPolicyName?: string;
  isProvisioningSession?: boolean;
  onAttentionDismiss?: (attention: GatewayDetailAttention) => void;
  onDeleted?: (gatewayName: string) => Promise<void> | void;
  onLeaveGuardChange?: (guard: ServiceAccountLeaveGuard | null) => void;
  onServiceAccountCollectionStateChange?: (
    collectionState: OpenShellGatewayServiceAccountListRequest,
    reason: ResourceTableStateChangeReason,
  ) => void;
  onTabChange?: (tab: GatewayDetailTab) => void;
  serviceAccountCollectionState?: OpenShellGatewayServiceAccountListRequest;
}

export function GatewayPage({
  activeTab,
  attention,
  gateway,
  gatewayId,
  initialPolicyName,
  isProvisioningSession = false,
  onAttentionDismiss,
  onDeleted,
  onLeaveGuardChange,
  onServiceAccountCollectionStateChange,
  onTabChange,
  serviceAccountCollectionState,
}: GatewayPageProps) {
  const intl = useIntl();
  const { gateways, navigation } = useGatewayUi();
  const trackConsoleWait = useConsoleWaitTracker();
  const [localTab, setLocalTab] = useState<GatewayDetailTab>("details");
  const currentTab = activeTab ?? localTab;
  // The service-accounts tab may register a guard while it holds an
  // unrecoverable one-time secret. Consulting it here prevents a tab switch from
  // silently unmounting the dialog and discarding that secret. The same guard is
  // forwarded to the host (via onLeaveGuardChange) so a route-level blocker can
  // also intercept SPA route changes and browser Back/Forward.
  const leaveGuardRef = useRef<ServiceAccountLeaveGuard | null>(null);
  const registerLeaveGuard = useCallback(
    (guard: ServiceAccountLeaveGuard | null) => {
      leaveGuardRef.current = guard;
      onLeaveGuardChange?.(guard);
    },
    [onLeaveGuardChange],
  );
  const changeTab = (tab: GatewayDetailTab) => {
    const applyTab = () => {
      if (onTabChange) {
        onTabChange(tab);
      } else {
        setLocalTab(tab);
      }
    };
    const guard = leaveGuardRef.current;
    if (tab !== currentTab && guard?.shouldBlock()) {
      // Cancelling keeps the current tab, so there is nothing to undo.
      const keepCurrentTab = () => undefined;
      guard.confirmLeave({ onCancel: keepCurrentTab, onConfirm: applyTab });
      return;
    }
    applyTab();
  };
  const [renamedGatewayName, setRenamedGatewayName] = useState<string>();
  const serviceAccountAttentionQuery = useQuery({
    enabled: gatewayId.length > 0,
    queryFn: ({ signal }) =>
      gateways.listOpenShellGatewayServiceAccounts(
        gatewayId,
        defaultOpenShellGatewayServiceAccountListRequest,
        signal,
      ),
    queryKey: serviceAccountListQueryKey(
      gatewayId,
      defaultOpenShellGatewayServiceAccountListRequest,
    ),
    refetchIntervalInBackground: false,
    retry: false,
    staleTime: 5_000,
  });
  const gatewayQuery = useQuery({
    enabled: gateway === undefined && gatewayId.length > 0,
    queryFn: ({ signal }) => gateways.getGateway(gatewayId, signal),
    queryKey: gatewayQueryKey(gatewayId),
    refetchInterval: ({ state }) => {
      if (!state.data) {
        return false;
      }
      return gatewayNeedsStatusPolling(
        state.data,
        trackConsoleWait(state.data.id, state.data),
      )
        ? gatewayStatusPollMilliseconds
        : false;
    },
    refetchIntervalInBackground: false,
  });
  // The Connection tab's sandbox command uses the fleet default resources
  // from the effective policies read (WEB-GLIS-11, WEB-GLIS-26.02).
  const connectionPoliciesQuery = useQuery({
    enabled:
      currentTab === "connection" &&
      gateways.getGatewayEffectivePolicies !== undefined &&
      gatewayId.length > 0,
    queryFn: ({ signal }) => {
      if (!gateways.getGatewayEffectivePolicies) {
        throw new Error("effective policies are unavailable");
      }
      return gateways.getGatewayEffectivePolicies(gatewayId, signal);
    },
    queryKey: gatewayEffectivePoliciesQueryKey(gatewayId),
    retry: false,
    staleTime: gatewayEffectivePoliciesStaleMilliseconds,
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
  // Anchor the console-ready deadline the detail header shows to when the UI
  // first observed this gateway awaiting its console, matching the polling clock
  // above rather than the gateway's createdAt.
  const consoleWaitStartedAt = trackConsoleWait(
    visibleGateway.id,
    visibleGateway,
  );
  const conditions = visibleGateway.provisioningConditions ?? [];
  const successfullyProvisioned =
    connection.phase?.toLocaleLowerCase() === "running" &&
    gatewayStatusAppearance(connection.status) === "success" &&
    conditions.every((condition) => condition.conditionStatus === "Complete");
  const showProvisioningStepper =
    isProvisioningSession || !successfullyProvisioned;
  const gatewayStatusTone = gatewayStatusAppearance(connection.status);
  const showGatewayHealthAttention =
    gatewayStatusTone === "warning" || gatewayStatusTone === "danger";
  const expiringServiceAccounts = (
    serviceAccountAttentionQuery.data?.items ?? []
  ).filter((account) => isServiceAccountExpiringSoon(account));
  const showServiceAccountAttention = expiringServiceAccounts.length > 0;

  return (
    <>
      <GatewaySuccessAlerts
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
        <GatewayDetailHeader
          consoleWaitStartedAt={consoleWaitStartedAt}
          gateway={connection}
          onDeleted={() => {
            if (onDeleted) {
              void onDeleted(connection.name);
            } else {
              void navigation.navigate(
                navigation.collectionHref,
                deletedGatewayNavigation(connection.name),
              );
            }
          }}
          onRenamed={setRenamedGatewayName}
        />
      </PageSection>
      {showProvisioningStepper && (
        <PageSection hasBodyWrapper={false}>
          <div className={styles.provisioningStepper}>
            <GatewayProvisioningStepper
              conditions={visibleGateway.provisioningConditions ?? []}
              consoleReady={Boolean(connection.consoleUrl)}
              phase={visibleGateway.phase}
            />
          </div>
        </PageSection>
      )}
      <PageSection
        data-page-region="body"
        hasBodyWrapper={false}
        isFilled
        variant="default"
      >
        <Tabs
          activeKey={currentTab}
          aria-label={intl.formatMessage(messages.connectionTabsLabel)}
          mountOnEnter
          onSelect={(_event, eventKey) => {
            changeTab(toGatewayDetailTab(String(eventKey)));
          }}
          unmountOnExit
        >
          <Tab
            eventKey="details"
            title={
              <TabTitleText>
                <FormattedMessage {...messages.detailsTab} />
              </TabTitleText>
            }
          >
            <Stack hasGutter>
              <StackItem>
                {showGatewayHealthAttention ? (
                  <Alert
                    actionClose={
                      attention === "gateway-health" ? (
                        <AlertActionCloseButton
                          aria-label={intl.formatMessage(
                            messages.dismissGatewayAttention,
                          )}
                          onClose={() => {
                            onAttentionDismiss?.("gateway-health");
                          }}
                        />
                      ) : undefined
                    }
                    actionLinks={
                      connection.consoleUrl ? (
                        <Button
                          component="a"
                          href={`${connection.consoleUrl}/sandboxes`}
                          isInline
                          rel="noopener noreferrer"
                          target="_blank"
                          variant="link"
                        >
                          <FormattedMessage {...messages.viewAllSandboxes} />
                        </Button>
                      ) : undefined
                    }
                    isInline
                    title={intl.formatMessage(messages.gatewayNeedsAttention)}
                    variant={
                      gatewayStatusTone === "danger" ? "danger" : "warning"
                    }
                  >
                    {typeof connection.activeSandboxCount === "number" ? (
                      <FormattedMessage
                        {...messages.gatewayHealthAttentionBodyWithCount}
                        values={{
                          activeSandboxCount: connection.activeSandboxCount,
                        }}
                      />
                    ) : (
                      <FormattedMessage
                        {...messages.gatewayHealthAttentionBody}
                      />
                    )}
                  </Alert>
                ) : null}
                {showServiceAccountAttention ? (
                  <Alert
                    actionClose={
                      attention === "service-account-expiring" ? (
                        <AlertActionCloseButton
                          aria-label={intl.formatMessage(
                            messages.dismissGatewayAttention,
                          )}
                          onClose={() => {
                            onAttentionDismiss?.("service-account-expiring");
                          }}
                        />
                      ) : undefined
                    }
                    actionLinks={
                      <Button
                        isInline
                        onClick={() => {
                          changeTab("service-accounts");
                        }}
                        variant="link"
                      >
                        <FormattedMessage {...messages.reviewServiceAccounts} />
                      </Button>
                    }
                    isInline
                    title={intl.formatMessage(
                      messages.serviceAccountExpiringTitle,
                      { count: expiringServiceAccounts.length },
                    )}
                    variant="warning"
                  >
                    <FormattedMessage
                      {...messages.serviceAccountExpiringBody}
                    />
                  </Alert>
                ) : null}
              </StackItem>
              <StackItem>
                <Flex
                  justifyContent={{ default: "justifyContentSpaceBetween" }}
                >
                  <FlexItem>
                    <Title headingLevel="h2" size="xl">
                      <FormattedMessage {...messages.details} />
                    </Title>
                    <Content component="p">
                      <FormattedMessage {...messages.detailsTabDescription} />
                    </Content>
                  </FlexItem>
                  <FlexItem>
                    <Button
                      aria-label={intl.formatMessage(
                        messages.refreshGatewayDetails,
                      )}
                      icon={<SyncAltIcon />}
                      onClick={() => void gatewayQuery.refetch()}
                      variant="plain"
                    />
                  </FlexItem>
                </Flex>
              </StackItem>
              <StackItem>
                <Card>
                  <CardHeader>
                    <CardTitle>
                      <Title headingLevel="h2" size="xl">
                        <FormattedMessage {...messages.overview} />
                      </Title>
                    </CardTitle>
                  </CardHeader>
                  <CardBody>
                    <Grid hasGutter>
                      <GridItem lg={6} span={12}>
                        <DescriptionList>
                          <DescriptionListGroup>
                            <DescriptionListTerm>
                              <FormattedMessage {...messages.status} />
                            </DescriptionListTerm>
                            <DescriptionListDescription>
                              <GatewayStatus status={connection.status} />
                            </DescriptionListDescription>
                          </DescriptionListGroup>
                          <DescriptionListGroup>
                            <DescriptionListTerm>
                              <FormattedMessage {...messages.sandboxes} />
                            </DescriptionListTerm>
                            <DescriptionListDescription>
                              {typeof connection.activeSandboxCount === "number"
                                ? connection.activeSandboxCount
                                : intl.formatMessage(messages.notAvailable)}
                            </DescriptionListDescription>
                          </DescriptionListGroup>
                          <DescriptionListGroup>
                            <DescriptionListTerm>
                              <FormattedMessage {...messages.clusterType} />
                            </DescriptionListTerm>
                            <DescriptionListDescription>
                              {connection.clusterType?.trim() ? (
                                connection.clusterType
                              ) : (
                                <FormattedMessage {...messages.notAvailable} />
                              )}
                            </DescriptionListDescription>
                          </DescriptionListGroup>
                        </DescriptionList>
                      </GridItem>
                      <GridItem lg={6} span={12}>
                        <DescriptionList>
                          <DescriptionListGroup>
                            <DescriptionListTerm>
                              <FormattedMessage {...messages.version} />
                            </DescriptionListTerm>
                            <DescriptionListDescription>
                              {connection.gatewayVersion?.trim() ? (
                                connection.gatewayVersion
                              ) : (
                                <FormattedMessage
                                  {...messages.versionPlaceholder}
                                />
                              )}
                            </DescriptionListDescription>
                          </DescriptionListGroup>
                          <DescriptionListGroup>
                            <DescriptionListTerm>
                              <FormattedMessage {...messages.created} />
                            </DescriptionListTerm>
                            <DescriptionListDescription>
                              <GatewayCreatedDate
                                createdAt={connection.createdAt}
                              />
                            </DescriptionListDescription>
                          </DescriptionListGroup>
                          <DescriptionListGroup>
                            <DescriptionListTerm>
                              <FormattedMessage {...messages.createdBy} />
                            </DescriptionListTerm>
                            <DescriptionListDescription>
                              {connection.createdBy?.trim() ? (
                                connection.createdBy
                              ) : (
                                <FormattedMessage {...messages.notAvailable} />
                              )}
                            </DescriptionListDescription>
                          </DescriptionListGroup>
                        </DescriptionList>
                      </GridItem>
                      <GridItem span={12}>
                        <Flex
                          alignItems={{ default: "alignItemsFlexEnd" }}
                          flexWrap={{ default: "wrap" }}
                          justifyContent={{
                            default: "justifyContentSpaceBetween",
                          }}
                        >
                          <FlexItem grow={{ default: "grow" }}>
                            <DescriptionList>
                              <DescriptionListGroup>
                                <DescriptionListTerm>
                                  <FormattedMessage
                                    {...messages.gatewayEndpoint}
                                  />
                                </DescriptionListTerm>
                                <DescriptionListDescription>
                                  {connection.endpoint ? (
                                    <div className={styles.endpointCopy}>
                                      <GatewayEndpointCopy
                                        gateway={connection}
                                      />
                                    </div>
                                  ) : (
                                    <FormattedMessage
                                      {...messages.notAvailable}
                                    />
                                  )}
                                </DescriptionListDescription>
                              </DescriptionListGroup>
                            </DescriptionList>
                          </FlexItem>
                          {connection.consoleUrl ? (
                            <FlexItem>
                              <Button
                                component="a"
                                href={connection.consoleUrl}
                                icon={<ExternalLinkAltIcon />}
                                iconPosition="end"
                                rel="noopener noreferrer"
                                target="_blank"
                                variant="secondary"
                              >
                                <FormattedMessage
                                  {...messages.openGatewayConsole}
                                />
                              </Button>
                            </FlexItem>
                          ) : null}
                        </Flex>
                      </GridItem>
                    </Grid>
                  </CardBody>
                </Card>
              </StackItem>
              <StackItem>
                <Grid hasGutter>
                  <GridItem lg={6} span={12}>
                    <Card>
                      <CardHeader>
                        <CardTitle>
                          <Title headingLevel="h2" size="xl">
                            <FormattedMessage {...messages.identity} />
                          </Title>
                        </CardTitle>
                      </CardHeader>
                      <CardBody>
                        <DescriptionList>
                          <DescriptionListGroup>
                            <DescriptionListTerm>
                              <FormattedMessage {...messages.gatewayId} />
                            </DescriptionListTerm>
                            <DescriptionListDescription>
                              {visibleGateway.id}
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
                        </DescriptionList>
                      </CardBody>
                    </Card>
                  </GridItem>
                  <GridItem lg={6} span={12}>
                    <Card>
                      <CardHeader>
                        <CardTitle>
                          <Title headingLevel="h2" size="xl">
                            <FormattedMessage {...messages.placement} />
                          </Title>
                        </CardTitle>
                      </CardHeader>
                      <CardBody>
                        <DescriptionList>
                          <DescriptionListGroup>
                            <DescriptionListTerm>
                              <FormattedMessage {...messages.cluster} />
                            </DescriptionListTerm>
                            <DescriptionListDescription>
                              <GatewayDetailClusterName gateway={connection} />
                            </DescriptionListDescription>
                          </DescriptionListGroup>
                        </DescriptionList>
                      </CardBody>
                    </Card>
                  </GridItem>
                </Grid>
              </StackItem>
            </Stack>
          </Tab>
          <Tab
            eventKey="connection"
            title={
              <TabTitleText>
                <FormattedMessage {...messages.connectionTab} />
              </TabTitleText>
            }
          >
            <Stack hasGutter>
              <StackItem>
                <Flex
                  justifyContent={{ default: "justifyContentSpaceBetween" }}
                >
                  <FlexItem>
                    <Title headingLevel="h2" size="xl">
                      <FormattedMessage {...messages.connectionTab} />
                    </Title>
                    <Content component="p">
                      <FormattedMessage
                        {...messages.connectionTabDescription}
                      />
                    </Content>
                  </FlexItem>
                  <FlexItem>
                    <Button
                      aria-label={intl.formatMessage(
                        messages.refreshGatewayDetails,
                      )}
                      icon={<SyncAltIcon />}
                      onClick={() => void gatewayQuery.refetch()}
                      variant="plain"
                    />
                  </FlexItem>
                </Flex>
              </StackItem>
              <StackItem>
                <GatewayConnectionSteps
                  gateway={connection}
                  sandboxResources={
                    connectionPoliciesQuery.data
                      ? sandboxResourcesFromDefaults(
                          connectionPoliciesQuery.data.defaults,
                        )
                      : undefined
                  }
                  isProvisioning={
                    !connection.phase ||
                    connection.phase.toLocaleLowerCase() === "pending" ||
                    connection.phase.toLocaleLowerCase() === "provisioning"
                  }
                />
              </StackItem>
              <StackItem>
                <Card>
                  <CardHeader>
                    <CardTitle>
                      <Title headingLevel="h2" size="xl">
                        <FormattedMessage {...messages.automateThisGateway} />
                      </Title>
                    </CardTitle>
                  </CardHeader>
                  <CardBody>
                    <Content component="p">
                      <FormattedMessage
                        {...messages.automateThisGatewayDescription}
                      />
                    </Content>
                    <Button
                      isInline
                      onClick={() => {
                        changeTab("service-accounts");
                      }}
                      variant="link"
                    >
                      <FormattedMessage {...messages.viewServiceAccounts} />
                    </Button>
                  </CardBody>
                </Card>
              </StackItem>
            </Stack>
          </Tab>
          <Tab
            eventKey="user-access"
            title={
              <TabTitleText>
                <FormattedMessage {...messages.userAccessTab} />
              </TabTitleText>
            }
          >
            <AccessPage
              gatewayId={gatewayId}
              isActive={currentTab === "user-access"}
              key={gatewayId}
            />
          </Tab>
          <Tab
            eventKey="service-accounts"
            title={
              <TabTitleText>
                <FormattedMessage {...messages.serviceAccountsTab} />
              </TabTitleText>
            }
          >
            <ServiceAccountsPage
              collectionState={serviceAccountCollectionState}
              expiringServiceAccounts={expiringServiceAccounts}
              gatewayId={gatewayId}
              isActive={currentTab === "service-accounts"}
              key={gatewayId}
              onCollectionStateChange={onServiceAccountCollectionStateChange}
              registerLeaveGuard={registerLeaveGuard}
            />
          </Tab>
          <Tab
            eventKey="policies"
            title={
              <TabTitleText>
                <FormattedMessage {...gatewayPoliciesMessages.tab} />
              </TabTitleText>
            }
          >
            <GatewayPoliciesTab
              gatewayId={gatewayId}
              gatewayName={connection.name}
              initialPolicyName={initialPolicyName}
              isActive={currentTab === "policies"}
              key={gatewayId}
            />
          </Tab>
        </Tabs>
      </PageSection>
    </>
  );
}
