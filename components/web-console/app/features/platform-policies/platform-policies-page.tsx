import {
  GatewayStatus,
  policyMessages,
  PolicyModeExplanationPopover,
  PolicyModeLabel,
  PolicyModeWithReason,
  policyResourceLabel,
  policyRuleResources,
  additionalRules,
  type PolicyMode,
} from "@openshift-online/hypershell-gateway-management-ui";
import {
  Alert,
  AlertActionLink,
  Button,
  Card,
  CardBody,
  CardFooter,
  CardHeader,
  CardTitle,
  Content,
  DescriptionList,
  DescriptionListDescription,
  DescriptionListGroup,
  DescriptionListTerm,
  EmptyState,
  EmptyStateActions,
  EmptyStateBody,
  EmptyStateFooter,
  Flex,
  FlexItem,
  Grid,
  GridItem,
  Label,
  LabelGroup,
  PageSection,
  SearchInput,
  Spinner,
  Stack,
  StackItem,
  Title,
  Toolbar,
  ToolbarContent,
  ToolbarItem,
} from "@patternfly/react-core";
import { ShieldAltIcon } from "@patternfly/react-icons";
import {
  ActionsColumn,
  Table,
  Tbody,
  Td,
  Th,
  Thead,
  Tr,
  type IAction,
} from "@patternfly/react-table";
import { useIntl } from "react-intl";

import type {
  PlatformPolicy,
  PlatformPolicySettings,
} from "../../application/platform-policies/platform-policy-types";
import { platformPoliciesMessages as messages } from "./platform-policies-messages";
import {
  navigationLinkProps,
  PlatformPolicyDialogs,
  PlatformPolicyWriteAlert,
} from "./platform-policy-dialogs";
import {
  affectedGateways,
  filterAndSortPolicies,
  gatewayLabel,
  policyTargetSummary,
  rolloutStatusToken,
  rolloutSummary,
  type PlatformPolicyDialog,
  type PlatformPolicyWriteFailure,
  type PolicySortDirection,
} from "./platform-policy-summary";

export interface PlatformPolicyLinks {
  createHref: string;
  detailHref: (policyId: string) => string;
  editHref: (policyId: string) => string;
  gatewaysHref: (policyId: string) => string;
  onNavigate: (href: string) => void;
}

export interface PlatformPoliciesPageProps {
  dialog?: PlatformPolicyDialog;
  filterText: string;
  isError: boolean;
  isLoading: boolean;
  isWriting: boolean;
  links: PlatformPolicyLinks;
  onConfirmDialog: (dialog: PlatformPolicyDialog) => void;
  onDialogChange: (dialog: PlatformPolicyDialog | undefined) => void;
  onDismissWriteFailure: () => void;
  onFilterTextChange: (text: string) => void;
  onRetry: () => void;
  onSortDirectionChange: (direction: PolicySortDirection) => void;
  policies: readonly PlatformPolicy[];
  settings?: PlatformPolicySettings;
  sortDirection: PolicySortDirection;
  writeFailure?: PlatformPolicyWriteFailure;
}

function RolloutStatus({ policy }: { policy: PlatformPolicy }) {
  const intl = useIntl();
  const rollout = rolloutSummary(policy);
  const problems = [
    rollout.unsupported
      ? intl.formatMessage(messages.cannotEnforceCount, {
          count: rollout.unsupported,
        })
      : undefined,
    rollout.pending
      ? intl.formatMessage(messages.pendingCount, { count: rollout.pending })
      : undefined,
  ].filter((problem): problem is string => problem !== undefined);
  return (
    <Stack>
      <StackItem>
        <GatewayStatus
          label={intl.formatMessage(messages.appliedOn, {
            applied: rollout.applied,
            total: rollout.total,
          })}
          status={rolloutStatusToken[rollout.appearance]}
        />
      </StackItem>
      {problems.length ? (
        <StackItem>
          <Content component="small">{intl.formatList(problems)}</Content>
        </StackItem>
      ) : null}
    </Stack>
  );
}

/** Rule chips: resources a policy bounds plus rules shown as Cedar. */
export function PolicyRuleChips({
  numLabels = 2,
  policy,
}: {
  numLabels?: number;
  policy: Pick<PlatformPolicy, "rendered">;
}): React.ReactNode {
  const intl = useIntl();
  const extra = additionalRules(policy.rendered);
  return (
    <LabelGroup numLabels={numLabels}>
      {policyRuleResources(policy.rendered).map((resource) => (
        <Label isCompact key={resource}>
          {policyResourceLabel(intl, resource)}
        </Label>
      ))}
      {extra.map((rule, index) => (
        <Label
          isCompact
          key={`${String(index)}-${rule.cedar}`}
          variant="outline"
        >
          {intl.formatMessage(policyMessages.additionalRule)}
        </Label>
      ))}
    </LabelGroup>
  );
}

export function policyModeReason(
  globalMode: PolicyMode,
): "platform" | "policy" {
  return globalMode === "enforce" ? "platform" : "policy";
}

function PoliciesTable({
  globalMode,
  links,
  onDialogChange,
  onSortDirectionChange,
  policies,
  sortDirection,
}: {
  globalMode: PolicyMode;
  links: PlatformPolicyLinks;
  onDialogChange: (dialog: PlatformPolicyDialog) => void;
  onSortDirectionChange: (direction: PolicySortDirection) => void;
  policies: readonly PlatformPolicy[];
  sortDirection: PolicySortDirection;
}) {
  const intl = useIntl();
  const globalEnforced = globalMode === "enforce";
  const heading = {
    actions: intl.formatMessage(messages.columnActions),
    applied: intl.formatMessage(messages.columnApplied),
    appliesTo: intl.formatMessage(messages.columnAppliesTo),
    enforcement: intl.formatMessage(messages.columnEnforcement),
    policy: intl.formatMessage(messages.columnPolicy),
    rules: intl.formatMessage(messages.columnRules),
    wouldReject: intl.formatMessage(messages.columnWouldReject),
  };
  const actions = (policy: PlatformPolicy): IAction[] => [
    policy.enforcementMode === "enforce"
      ? {
          description: globalEnforced
            ? intl.formatMessage(messages.returnToAuditDisabled)
            : undefined,
          isAriaDisabled: globalEnforced,
          onClick: () => {
            onDialogChange({
              kind: "policy-mode",
              policyId: policy.id,
              to: "audit",
            });
          },
          title: intl.formatMessage(messages.returnToAudit),
        }
      : {
          description: globalEnforced
            ? intl.formatMessage(messages.startEnforcingDisabled)
            : undefined,
          isAriaDisabled: globalEnforced,
          onClick: () => {
            onDialogChange({
              kind: "policy-mode",
              policyId: policy.id,
              to: "enforce",
            });
          },
          title: intl.formatMessage(messages.startEnforcing),
        },
    {
      onClick: () => {
        links.onNavigate(links.editHref(policy.id));
      },
      title: intl.formatMessage(messages.editPolicy),
    },
    { isSeparator: true },
    {
      description: policy.builtIn
        ? intl.formatMessage(messages.builtInCannotDelete)
        : undefined,
      isAriaDisabled: policy.builtIn,
      isDanger: true,
      onClick: () => {
        onDialogChange({ kind: "delete", policyId: policy.id });
      },
      title: intl.formatMessage(messages.deletePolicy),
    },
  ];

  return (
    <Table aria-label={intl.formatMessage(messages.tableLabel)}>
      <Thead>
        <Tr>
          <Th
            sort={{
              columnIndex: 0,
              onSort: (_event, _index, direction) => {
                onSortDirectionChange(direction);
              },
              sortBy: { direction: sortDirection, index: 0 },
            }}
            width={20}
          >
            {heading.policy}
          </Th>
          <Th width={15}>{heading.appliesTo}</Th>
          <Th width={20}>{heading.rules}</Th>
          <Th width={15}>{heading.enforcement}</Th>
          <Th width={20}>{heading.applied}</Th>
          <Th modifier="wrap" width={10}>
            {heading.wouldReject}
          </Th>
          <Th screenReaderText={heading.actions} />
        </Tr>
      </Thead>
      <Tbody>
        {policies.map((policy) => {
          const enforced = policy.effectiveMode === "enforce";
          return (
            <Tr key={policy.id}>
              <Td dataLabel={heading.policy}>
                <Button
                  component="a"
                  isInline
                  variant="link"
                  {...navigationLinkProps(
                    links.detailHref(policy.id),
                    links.onNavigate,
                  )}
                >
                  {policy.name}
                </Button>
              </Td>
              <Td dataLabel={heading.appliesTo}>
                {intl.formatMessage(
                  messages.appliesTo,
                  policyTargetSummary(policy.target),
                )}
              </Td>
              <Td dataLabel={heading.rules}>
                <PolicyRuleChips policy={policy} />
              </Td>
              <Td dataLabel={heading.enforcement}>
                <PolicyModeWithReason
                  enforced={enforced}
                  reason={intl.formatMessage(
                    globalEnforced
                      ? policyMessages.reasonPlatformEnforced
                      : policyMessages.reasonPolicySetting,
                  )}
                />
              </Td>
              <Td dataLabel={heading.applied}>
                <RolloutStatus policy={policy} />
              </Td>
              <Td dataLabel={heading.wouldReject}>
                {enforced
                  ? intl.formatMessage(messages.notApplicable)
                  : policy.findingsLast24h.wouldReject}
              </Td>
              <Td isActionCell>
                <ActionsColumn
                  items={actions(policy)}
                  popperProps={{ position: "right" }}
                />
              </Td>
            </Tr>
          );
        })}
      </Tbody>
    </Table>
  );
}

function CardHeading({ children }: { children: React.ReactNode }) {
  return (
    <CardHeader>
      <CardTitle>
        <Flex
          alignItems={{ default: "alignItemsCenter" }}
          gap={{ default: "gapSm" }}
        >
          {children}
        </Flex>
      </CardTitle>
    </CardHeader>
  );
}

/**
 * Platform policies list with the global enforcement toggle (WEB-PADM-02/03).
 * A pure view: the route container supplies loaded data and callbacks.
 */
export function PlatformPoliciesPage({
  dialog,
  filterText,
  isError,
  isLoading,
  isWriting,
  links,
  onConfirmDialog,
  onDialogChange,
  onDismissWriteFailure,
  onFilterTextChange,
  onRetry,
  onSortDirectionChange,
  policies,
  settings,
  sortDirection,
  writeFailure,
}: PlatformPoliciesPageProps): React.ReactNode {
  const intl = useIntl();
  const globalMode: PolicyMode = settings?.mode ?? "audit";
  const globalEnforced = globalMode === "enforce";
  const isEmpty = !isLoading && !isError && policies.length === 0;
  const visible = filterAndSortPolicies(policies, filterText, sortDirection);
  const unsupported = policies.flatMap((policy) => {
    const gateways = policy.gateways.filter(
      ({ condition }) => condition === "unsupported",
    );
    return gateways.length ? [{ gateways, policy }] : [];
  });
  const auditPolicies = policies.filter(
    ({ effectiveMode }) => effectiveMode === "audit",
  );
  const wouldReject = auditPolicies.reduce(
    (total, policy) => total + policy.findingsLast24h.wouldReject,
    0,
  );
  const wouldAddDefaults = auditPolicies.reduce(
    (total, policy) => total + policy.findingsLast24h.wouldAddDefaults,
    0,
  );
  const gateways = affectedGateways(policies);
  const noAuditFindings = globalEnforced || auditPolicies.length === 0;

  let body: React.ReactNode;
  if (isLoading) {
    body = <Spinner aria-label={intl.formatMessage(messages.loading)} />;
  } else if (isError) {
    body = (
      <EmptyState
        headingLevel="h2"
        titleText={intl.formatMessage(messages.loadErrorTitle)}
      >
        <EmptyStateBody>
          {intl.formatMessage(messages.loadErrorBody)}
        </EmptyStateBody>
        <EmptyStateFooter>
          <EmptyStateActions>
            <Button onClick={onRetry} variant="primary">
              {intl.formatMessage(messages.retry)}
            </Button>
          </EmptyStateActions>
        </EmptyStateFooter>
      </EmptyState>
    );
  } else {
    body = (
      <Grid hasGutter>
        {writeFailure ? (
          <GridItem>
            <PlatformPolicyWriteAlert
              failure={writeFailure}
              onDismiss={onDismissWriteFailure}
            />
          </GridItem>
        ) : null}
        {unsupported.map(({ gateways: blocked, policy }) => {
          const [first] = blocked;
          return (
            <GridItem key={policy.id}>
              <Alert
                actionLinks={
                  <AlertActionLink
                    component="a"
                    {...navigationLinkProps(
                      links.gatewaysHref(policy.id),
                      links.onNavigate,
                    )}
                  >
                    {intl.formatMessage(messages.reviewRollout)}
                  </AlertActionLink>
                }
                isInline
                title={intl.formatMessage(messages.unsupportedAlertTitle, {
                  count: blocked.length,
                  name: policy.name,
                })}
                variant="warning"
              >
                {first?.message
                  ? intl.formatMessage(messages.unsupportedAlertBody, {
                      gateway: gatewayLabel(first),
                      message: first.message,
                    })
                  : intl.formatMessage(messages.unsupportedAlertBodyNoReason, {
                      gateway: first ? gatewayLabel(first) : "",
                    })}
              </Alert>
            </GridItem>
          );
        })}
        <GridItem lg={6}>
          <Card isFullHeight>
            <CardHeading>
              <FlexItem>
                <Title headingLevel="h2" size="lg">
                  {intl.formatMessage(policyMessages.platformEnforcement)}
                </Title>
              </FlexItem>
              <FlexItem>
                <PolicyModeLabel enforced={globalEnforced} />
              </FlexItem>
              <FlexItem>
                <PolicyModeExplanationPopover globalMode={globalMode} />
              </FlexItem>
            </CardHeading>
            <CardBody>
              <Stack hasGutter>
                <StackItem>
                  <Content component="p">
                    {intl.formatMessage(
                      globalEnforced
                        ? messages.enforcementDescriptionEnforce
                        : messages.enforcementDescriptionAudit,
                    )}
                  </Content>
                </StackItem>
                {globalEnforced ? null : (
                  <StackItem>
                    <DescriptionList isCompact isHorizontal>
                      <DescriptionListGroup>
                        <DescriptionListTerm>
                          {intl.formatMessage(messages.enforcedByPolicySetting)}
                        </DescriptionListTerm>
                        <DescriptionListDescription>
                          {intl.formatMessage(messages.policiesOfTotal, {
                            count: policies.filter(
                              ({ enforcementMode }) =>
                                enforcementMode === "enforce",
                            ).length,
                            total: policies.length,
                          })}
                        </DescriptionListDescription>
                      </DescriptionListGroup>
                    </DescriptionList>
                  </StackItem>
                )}
              </Stack>
            </CardBody>
            <CardFooter>
              <Button
                isDisabled={!settings}
                onClick={() => {
                  onDialogChange({
                    kind: "global-mode",
                    to: globalEnforced ? "audit" : "enforce",
                  });
                }}
                variant="secondary"
              >
                {intl.formatMessage(
                  globalEnforced
                    ? messages.switchToAudit
                    : messages.switchToEnforced,
                )}
              </Button>
            </CardFooter>
          </Card>
        </GridItem>
        <GridItem lg={6}>
          <Card isFullHeight>
            <CardHeading>
              <FlexItem>
                <Title headingLevel="h2" size="lg">
                  {intl.formatMessage(messages.findingsTitle)}
                </Title>
              </FlexItem>
              {!noAuditFindings && wouldReject > 0 ? (
                <FlexItem>
                  <GatewayStatus
                    label={intl.formatMessage(
                      messages.findingsWouldBeRejected,
                      { count: wouldReject },
                    )}
                    status="warning"
                  />
                </FlexItem>
              ) : null}
            </CardHeading>
            <CardBody>
              {noAuditFindings ? (
                <Content component="p">
                  {intl.formatMessage(messages.findingsNothing)}
                </Content>
              ) : (
                <DescriptionList
                  columnModifier={{ default: "1Col", md: "3Col" }}
                  displaySize="lg"
                >
                  <DescriptionListGroup>
                    <DescriptionListTerm>
                      {intl.formatMessage(messages.findingsWouldReject)}
                    </DescriptionListTerm>
                    <DescriptionListDescription>
                      {intl.formatMessage(messages.operationsCount, {
                        count: wouldReject,
                      })}
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                  <DescriptionListGroup>
                    <DescriptionListTerm>
                      {intl.formatMessage(messages.findingsWouldAddDefaults)}
                    </DescriptionListTerm>
                    <DescriptionListDescription>
                      {intl.formatMessage(messages.operationsCount, {
                        count: wouldAddDefaults,
                      })}
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                  <DescriptionListGroup>
                    <DescriptionListTerm>
                      {intl.formatMessage(messages.findingsGatewaysAffected)}
                    </DescriptionListTerm>
                    <DescriptionListDescription>
                      {intl.formatMessage(messages.countOfTotal, {
                        count: gateways.affected,
                        total: gateways.total,
                      })}
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                </DescriptionList>
              )}
            </CardBody>
          </Card>
        </GridItem>
        <GridItem>
          <Card>
            <CardHeader>
              <CardTitle>
                <Title headingLevel="h2" size="lg">
                  {intl.formatMessage(messages.policiesTitle)}
                </Title>
              </CardTitle>
            </CardHeader>
            <CardBody>
              {isEmpty ? (
                <EmptyState
                  headingLevel="h3"
                  icon={ShieldAltIcon}
                  titleText={intl.formatMessage(messages.emptyTitle)}
                >
                  <EmptyStateBody>
                    {intl.formatMessage(messages.emptyBody)}
                  </EmptyStateBody>
                  <EmptyStateFooter>
                    <EmptyStateActions>
                      <Button
                        component="a"
                        variant="primary"
                        {...navigationLinkProps(
                          links.createHref,
                          links.onNavigate,
                        )}
                      >
                        {intl.formatMessage(messages.createPolicy)}
                      </Button>
                    </EmptyStateActions>
                  </EmptyStateFooter>
                </EmptyState>
              ) : (
                <Stack hasGutter>
                  <StackItem>
                    <Toolbar>
                      <ToolbarContent>
                        <ToolbarItem>
                          <SearchInput
                            aria-label={intl.formatMessage(
                              messages.filterLabel,
                            )}
                            onChange={(_event, value) => {
                              onFilterTextChange(value);
                            }}
                            onClear={() => {
                              onFilterTextChange("");
                            }}
                            placeholder={intl.formatMessage(
                              messages.filterPlaceholder,
                            )}
                            value={filterText}
                          />
                        </ToolbarItem>
                      </ToolbarContent>
                    </Toolbar>
                  </StackItem>
                  <StackItem>
                    {visible.length ? (
                      <PoliciesTable
                        globalMode={globalMode}
                        links={links}
                        onDialogChange={onDialogChange}
                        onSortDirectionChange={onSortDirectionChange}
                        policies={visible}
                        sortDirection={sortDirection}
                      />
                    ) : (
                      <EmptyState
                        headingLevel="h3"
                        titleText={intl.formatMessage(messages.noMatchesTitle)}
                        variant="sm"
                      >
                        <EmptyStateFooter>
                          <EmptyStateActions>
                            <Button
                              onClick={() => {
                                onFilterTextChange("");
                              }}
                              variant="link"
                            >
                              {intl.formatMessage(messages.clearFilter)}
                            </Button>
                          </EmptyStateActions>
                        </EmptyStateFooter>
                      </EmptyState>
                    )}
                  </StackItem>
                </Stack>
              )}
            </CardBody>
          </Card>
        </GridItem>
      </Grid>
    );
  }

  return (
    <>
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
              {intl.formatMessage(messages.pageTitle)}
            </Title>
            <Content component="p">
              {intl.formatMessage(messages.pageDescription)}
            </Content>
          </FlexItem>
          {isEmpty ? null : (
            <FlexItem>
              <Button
                component="a"
                variant="primary"
                {...navigationLinkProps(links.createHref, links.onNavigate)}
              >
                {intl.formatMessage(messages.createPolicy)}
              </Button>
            </FlexItem>
          )}
        </Flex>
      </PageSection>
      <PageSection
        data-page-region="body"
        hasBodyWrapper={false}
        isFilled
        variant="secondary"
      >
        {body}
      </PageSection>
      <PlatformPolicyDialogs
        dialog={dialog}
        isWriting={isWriting}
        onClose={() => {
          onDialogChange(undefined);
        }}
        onConfirm={onConfirmDialog}
        policies={policies}
      />
    </>
  );
}
