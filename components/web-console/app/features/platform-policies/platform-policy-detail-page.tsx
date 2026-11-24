import {
  additionalRules,
  formatPolicyTime,
  GatewayStatus,
  policyFindingDetail,
  policyMessages,
  CedarCodeBlock,
  PolicyCedarRule,
  PolicyModeLabel,
  PolicyModeWithReason,
  PolicyOutcomeLabel,
  policyRuleResources,
  PolicyRulesTable,
  type PolicyFinding,
  type PolicyFindingOutcome,
  type PolicyMode,
  type PolicyRolloutCondition,
} from "@openshift-online/hypershell-gateway-management-ui";
import {
  ActionList,
  ActionListItem,
  Button,
  Content,
  DescriptionList,
  DescriptionListDescription,
  DescriptionListGroup,
  DescriptionListTerm,
  EmptyState,
  EmptyStateActions,
  EmptyStateBody,
  EmptyStateFooter,
  ExpandableSection,
  Flex,
  FlexItem,
  FormSelect,
  FormSelectOption,
  Grid,
  GridItem,
  PageSection,
  Pagination,
  Spinner,
  Stack,
  StackItem,
  Tab,
  Tabs,
  TabTitleText,
  Title,
  ToggleGroup,
  ToggleGroupItem,
  Toolbar,
  ToolbarContent,
  ToolbarItem,
} from "@patternfly/react-core";
import { ExternalLinkAltIcon } from "@patternfly/react-icons";
import { Table, Tbody, Td, Th, Thead, Tr } from "@patternfly/react-table";
import { useState } from "react";
import { useIntl } from "react-intl";

import type {
  PlatformPolicy,
  PlatformPolicyGatewayStatus,
  PlatformPolicySettings,
} from "../../application/platform-policies/platform-policy-types";
import { platformPoliciesMessages as messages } from "./platform-policies-messages";
import {
  navigationLinkProps,
  PlatformPolicyDialogs,
  PlatformPolicyWriteAlert,
} from "./platform-policy-dialogs";
import {
  gatewayLabel,
  policyTargetSummary,
  rolloutStatusToken,
  rolloutSummary,
  type PlatformPolicyDialog,
  type PlatformPolicyWriteFailure,
  type RolloutAppearance,
} from "./platform-policy-summary";

export type PlatformPolicyDetailTab = "findings" | "gateways" | "overview";

export function toPlatformPolicyDetailTab(
  value: string | null | undefined,
): PlatformPolicyDetailTab {
  return value === "gateways" || value === "findings" ? value : "overview";
}

export interface PlatformPolicyFindingsFilter {
  gatewayId?: string;
  outcome?: PolicyFindingOutcome;
}

export interface PlatformPolicyFindingsView {
  isError: boolean;
  isLoading: boolean;
  items: readonly PolicyFinding[];
  page: number;
  size: number;
  total: number;
}

export interface PlatformPolicyDetailLinks {
  editHref: (policyId: string) => string;
  gatewayHref: (gatewayId: string) => string;
  listHref: string;
  onNavigate: (href: string) => void;
}

export interface PlatformPolicyDetailPageProps {
  activeTab: PlatformPolicyDetailTab;
  dialog?: PlatformPolicyDialog;
  findings: PlatformPolicyFindingsView;
  findingsFilter: PlatformPolicyFindingsFilter;
  isError: boolean;
  isLoading: boolean;
  isNotFound: boolean;
  isWriting: boolean;
  links: PlatformPolicyDetailLinks;
  onConfirmDialog: (dialog: PlatformPolicyDialog) => void;
  onDialogChange: (dialog: PlatformPolicyDialog | undefined) => void;
  onDismissWriteFailure: () => void;
  onFindingsFilterChange: (filter: PlatformPolicyFindingsFilter) => void;
  onFindingsPageChange: (page: number, size: number) => void;
  onRetry: () => void;
  onTabChange: (tab: PlatformPolicyDetailTab) => void;
  policy?: PlatformPolicy;
  settings?: PlatformPolicySettings;
  writeFailure?: PlatformPolicyWriteFailure;
}

const conditionPresentation: Readonly<
  Record<
    PolicyRolloutCondition,
    {
      appearance: RolloutAppearance;
      label: keyof typeof messages;
    }
  >
> = {
  applied: { appearance: "good", label: "rolloutApplied" },
  pending: { appearance: "warning", label: "rolloutPending" },
  unsupported: { appearance: "danger", label: "rolloutUnsupported" },
};

function OverviewTab({
  globalMode,
  policy,
}: {
  globalMode: PolicyMode;
  policy: PlatformPolicy;
}) {
  const intl = useIntl();
  const [isCedarExpanded, setIsCedarExpanded] = useState(false);
  const rollout = rolloutSummary(policy);
  const unrecognized = additionalRules(policy.rendered);
  return (
    <Grid hasGutter>
      <GridItem lg={7}>
        <Stack hasGutter>
          <StackItem>
            <Title headingLevel="h2" size="xl">
              {intl.formatMessage(messages.rulesTitle)}
            </Title>
          </StackItem>
          <StackItem>
            {policyRuleResources(policy.rendered).length ? (
              <PolicyRulesTable name={policy.name} rules={policy.rendered} />
            ) : unrecognized.length ? null : (
              <Content component="p">
                {intl.formatMessage(messages.noRecognizedRules)}
              </Content>
            )}
          </StackItem>
          {unrecognized.map((rule, index) => (
            <StackItem key={`${String(index)}-${rule.cedar}`}>
              <PolicyCedarRule rule={rule} />
            </StackItem>
          ))}
          <StackItem>
            <ExpandableSection
              isExpanded={isCedarExpanded}
              onToggle={(_event, expanded) => {
                setIsCedarExpanded(expanded);
              }}
              toggleText={intl.formatMessage(messages.cedarSourceTitle)}
            >
              <CedarCodeBlock source={policy.cedar} />
            </ExpandableSection>
          </StackItem>
          <StackItem>
            <Title headingLevel="h2" size="xl">
              {intl.formatMessage(policyMessages.messageForUsers)}
            </Title>
            <Content component="blockquote">{policy.userMessage}</Content>
            {policy.documentationUrl ? (
              <Button
                component="a"
                href={policy.documentationUrl}
                icon={<ExternalLinkAltIcon />}
                iconPosition="end"
                isInline
                rel="noopener noreferrer"
                target="_blank"
                variant="link"
              >
                {intl.formatMessage(policyMessages.policyDocumentation)}
              </Button>
            ) : null}
          </StackItem>
        </Stack>
      </GridItem>
      <GridItem lg={5}>
        <DescriptionList>
          <DescriptionListGroup>
            <DescriptionListTerm>
              {intl.formatMessage(messages.columnAppliesTo)}
            </DescriptionListTerm>
            <DescriptionListDescription>
              {intl.formatMessage(
                messages.appliesTo,
                policyTargetSummary(policy.target),
              )}
            </DescriptionListDescription>
          </DescriptionListGroup>
          <DescriptionListGroup>
            <DescriptionListTerm>
              {intl.formatMessage(messages.columnEnforcement)}
            </DescriptionListTerm>
            <DescriptionListDescription>
              {intl.formatMessage(messages.enforcementValue, {
                mode: policy.effectiveMode,
                reason: globalMode === "enforce" ? "platform" : "policy",
              })}
            </DescriptionListDescription>
          </DescriptionListGroup>
          <DescriptionListGroup>
            <DescriptionListTerm>
              {intl.formatMessage(messages.columnApplied)}
            </DescriptionListTerm>
            <DescriptionListDescription>
              <GatewayStatus
                label={intl.formatMessage(messages.appliedOn, {
                  applied: rollout.applied,
                  total: rollout.total,
                })}
                status={rolloutStatusToken[rollout.appearance]}
              />
            </DescriptionListDescription>
          </DescriptionListGroup>
          {policy.updatedAt ? (
            <DescriptionListGroup>
              <DescriptionListTerm>
                {intl.formatMessage(messages.lastUpdated)}
              </DescriptionListTerm>
              <DescriptionListDescription>
                {formatPolicyTime(intl, policy.updatedAt)}
              </DescriptionListDescription>
            </DescriptionListGroup>
          ) : null}
        </DescriptionList>
      </GridItem>
    </Grid>
  );
}

function GatewaysTab({
  globalMode,
  links,
  policy,
}: {
  globalMode: PolicyMode;
  links: PlatformPolicyDetailLinks;
  policy: PlatformPolicy;
}) {
  const intl = useIntl();
  if (policy.gateways.length === 0) {
    return (
      <Content component="p">{intl.formatMessage(messages.noGateways)}</Content>
    );
  }
  const heading = {
    applied: intl.formatMessage(messages.columnApplied),
    detail: intl.formatMessage(messages.columnDetail),
    enforcement: intl.formatMessage(messages.columnEnforcement),
    gateway: intl.formatMessage(messages.columnGateway),
    revision: intl.formatMessage(messages.columnRevision),
    wouldReject: intl.formatMessage(messages.columnWouldReject),
  };
  const notApplicable = intl.formatMessage(messages.notApplicable);
  const reason = (gateway: PlatformPolicyGatewayStatus) => {
    if (gateway.condition === "unsupported") {
      return intl.formatMessage(policyMessages.reasonCannotEnforce);
    }
    return intl.formatMessage(
      globalMode === "enforce"
        ? policyMessages.reasonPlatformEnforced
        : policyMessages.reasonPolicySetting,
    );
  };
  return (
    <Table aria-label={intl.formatMessage(messages.gatewaysTableLabel)}>
      <Thead>
        <Tr>
          <Th width={20}>{heading.gateway}</Th>
          <Th width={15}>{heading.applied}</Th>
          <Th width={10}>{heading.revision}</Th>
          <Th width={20}>{heading.enforcement}</Th>
          <Th modifier="wrap" width={10}>
            {heading.wouldReject}
          </Th>
          <Th width={25}>{heading.detail}</Th>
        </Tr>
      </Thead>
      <Tbody>
        {policy.gateways.map((gateway) => {
          const presentation = conditionPresentation[gateway.condition];
          const enforced =
            gateway.condition !== "unsupported" &&
            gateway.effectiveMode === "enforce";
          return (
            <Tr key={gateway.gatewayId}>
              <Td dataLabel={heading.gateway}>
                {gateway.gatewayName ? (
                  <Button
                    component="a"
                    isInline
                    variant="link"
                    {...navigationLinkProps(
                      links.gatewayHref(gateway.gatewayId),
                      links.onNavigate,
                    )}
                  >
                    {gateway.gatewayName}
                  </Button>
                ) : (
                  gateway.gatewayId
                )}
              </Td>
              <Td dataLabel={heading.applied}>
                <GatewayStatus
                  label={intl.formatMessage(messages[presentation.label])}
                  status={rolloutStatusToken[presentation.appearance]}
                />
              </Td>
              <Td dataLabel={heading.revision}>
                {gateway.revision ?? notApplicable}
              </Td>
              <Td dataLabel={heading.enforcement}>
                <PolicyModeWithReason
                  enforced={enforced}
                  reason={reason(gateway)}
                />
              </Td>
              <Td dataLabel={heading.wouldReject}>
                {enforced ? notApplicable : gateway.wouldReject24h}
              </Td>
              <Td dataLabel={heading.detail}>
                {gateway.message ?? notApplicable}
              </Td>
            </Tr>
          );
        })}
      </Tbody>
    </Table>
  );
}

function FindingsTab({
  findings,
  filter,
  onFilterChange,
  onPageChange,
  policy,
}: {
  filter: PlatformPolicyFindingsFilter;
  findings: PlatformPolicyFindingsView;
  onFilterChange: (filter: PlatformPolicyFindingsFilter) => void;
  onPageChange: (page: number, size: number) => void;
  policy: PlatformPolicy;
}) {
  const intl = useIntl();
  const names = new Map(
    policy.gateways.map((gateway) => [
      gateway.gatewayId,
      gatewayLabel(gateway),
    ]),
  );
  const heading = {
    detail: intl.formatMessage(messages.columnDetail),
    gateway: intl.formatMessage(messages.columnGateway),
    outcome: intl.formatMessage(messages.columnOutcome),
    time: intl.formatMessage(messages.columnTime),
    user: intl.formatMessage(messages.columnUser),
  };
  const outcomes: readonly (readonly [
    PolicyFindingOutcome | undefined,
    string,
  ])[] = [
    [undefined, intl.formatMessage(messages.findingsAllOutcomes)],
    ["would-reject", intl.formatMessage(policyMessages.outcomeWouldReject)],
    [
      "would-add-defaults",
      intl.formatMessage(policyMessages.outcomeWouldAddDefaults),
    ],
  ];

  let content: React.ReactNode;
  if (findings.isLoading) {
    content = (
      <Spinner aria-label={intl.formatMessage(messages.findingsLoading)} />
    );
  } else if (findings.isError) {
    content = (
      <Content component="p">
        {intl.formatMessage(messages.findingsError)}
      </Content>
    );
  } else if (findings.items.length === 0) {
    content = (
      <EmptyState
        headingLevel="h3"
        titleText={intl.formatMessage(messages.findingsEmptyTitle)}
        variant="xs"
      >
        <EmptyStateBody>
          {intl.formatMessage(messages.findingsEmptyBody)}
        </EmptyStateBody>
      </EmptyState>
    );
  } else {
    content = (
      <Table aria-label={intl.formatMessage(messages.findingsTableLabel)}>
        <Thead>
          <Tr>
            <Th width={20}>{heading.time}</Th>
            <Th width={20}>{heading.gateway}</Th>
            <Th width={15}>{heading.user}</Th>
            <Th width={15}>{heading.outcome}</Th>
            <Th width={30}>{heading.detail}</Th>
          </Tr>
        </Thead>
        <Tbody>
          {findings.items.map((finding) => (
            <Tr key={finding.id}>
              <Td dataLabel={heading.time}>
                {formatPolicyTime(intl, finding.occurredAt)}
              </Td>
              <Td dataLabel={heading.gateway}>
                {names.get(finding.gatewayId) ?? finding.gatewayId}
              </Td>
              <Td dataLabel={heading.user}>{finding.principalSubject}</Td>
              <Td dataLabel={heading.outcome}>
                <PolicyOutcomeLabel outcome={finding.outcome} />
              </Td>
              <Td dataLabel={heading.detail}>
                {policyFindingDetail(intl, finding)}
              </Td>
            </Tr>
          ))}
        </Tbody>
      </Table>
    );
  }

  return (
    <Stack hasGutter>
      <StackItem>
        <Toolbar>
          <ToolbarContent>
            <ToolbarItem>
              <FormSelect
                aria-label={intl.formatMessage(messages.findingsGatewayFilter)}
                id="platform-policy-findings-gateway"
                onChange={(_event, value) => {
                  onFilterChange({
                    ...filter,
                    gatewayId: value === "" ? undefined : value,
                  });
                }}
                value={filter.gatewayId ?? ""}
              >
                <FormSelectOption
                  label={intl.formatMessage(messages.findingsAllGateways)}
                  value=""
                />
                {policy.gateways.map((gateway) => (
                  <FormSelectOption
                    key={gateway.gatewayId}
                    label={gatewayLabel(gateway)}
                    value={gateway.gatewayId}
                  />
                ))}
              </FormSelect>
            </ToolbarItem>
            <ToolbarItem>
              <ToggleGroup
                aria-label={intl.formatMessage(messages.findingsOutcomeFilter)}
              >
                {outcomes.map(([value, label]) => (
                  <ToggleGroupItem
                    buttonId={`platform-policy-findings-${value ?? "all"}`}
                    isSelected={filter.outcome === value}
                    key={value ?? "all"}
                    onChange={() => {
                      onFilterChange({ ...filter, outcome: value });
                    }}
                    text={label}
                  />
                ))}
              </ToggleGroup>
            </ToolbarItem>
            {findings.total > 0 ? (
              <ToolbarItem variant="pagination">
                <Pagination
                  isCompact
                  itemCount={findings.total}
                  onPerPageSelect={(_event, size) => {
                    onPageChange(1, size);
                  }}
                  onSetPage={(_event, page) => {
                    onPageChange(page, findings.size);
                  }}
                  page={findings.page}
                  perPage={findings.size}
                />
              </ToolbarItem>
            ) : null}
          </ToolbarContent>
        </Toolbar>
      </StackItem>
      <StackItem>{content}</StackItem>
    </Stack>
  );
}

/**
 * Policy detail: rendered rules beside the Cedar source, message, per-gateway
 * rollout including Unsupported gateways, and filtered audit findings
 * (WEB-PADM-04). A pure view: the route container supplies data and callbacks.
 */
export function PlatformPolicyDetailPage({
  activeTab,
  dialog,
  findings,
  findingsFilter,
  isError,
  isLoading,
  isNotFound,
  isWriting,
  links,
  onConfirmDialog,
  onDialogChange,
  onDismissWriteFailure,
  onFindingsFilterChange,
  onFindingsPageChange,
  onRetry,
  onTabChange,
  policy,
  settings,
  writeFailure,
}: PlatformPolicyDetailPageProps) {
  const intl = useIntl();
  const globalMode: PolicyMode = settings?.mode ?? "audit";

  if (isLoading) {
    return (
      <PageSection hasBodyWrapper={false} isFilled>
        <Spinner aria-label={intl.formatMessage(messages.loading)} />
      </PageSection>
    );
  }
  if (!policy || isError) {
    return (
      <PageSection hasBodyWrapper={false} isFilled>
        <EmptyState
          headingLevel="h1"
          titleText={intl.formatMessage(
            isNotFound ? messages.notFoundTitle : messages.loadErrorTitle,
          )}
        >
          <EmptyStateBody>
            {intl.formatMessage(
              isNotFound ? messages.notFoundBody : messages.loadErrorBody,
            )}
          </EmptyStateBody>
          <EmptyStateFooter>
            <EmptyStateActions>
              {isNotFound ? null : (
                <Button onClick={onRetry} variant="primary">
                  {intl.formatMessage(messages.retry)}
                </Button>
              )}
              <Button
                component="a"
                variant="link"
                {...navigationLinkProps(links.listHref, links.onNavigate)}
              >
                {intl.formatMessage(messages.backToPolicies)}
              </Button>
            </EmptyStateActions>
          </EmptyStateFooter>
        </EmptyState>
      </PageSection>
    );
  }

  const globalEnforced = globalMode === "enforce";
  const policyEnforcing = policy.enforcementMode === "enforce";

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
            <Flex
              alignItems={{ default: "alignItemsCenter" }}
              gap={{ default: "gapMd" }}
            >
              <FlexItem>
                <Title headingLevel="h1" size="2xl">
                  {policy.name}
                </Title>
              </FlexItem>
              <FlexItem>
                <PolicyModeLabel
                  enforced={policy.effectiveMode === "enforce"}
                />
              </FlexItem>
            </Flex>
            <Content component="p">
              {intl.formatMessage(
                messages.appliesToSentence,
                policyTargetSummary(policy.target),
              )}
            </Content>
          </FlexItem>
          <FlexItem>
            <ActionList>
              <ActionListItem>
                <Button
                  isAriaDisabled={globalEnforced}
                  onClick={() => {
                    onDialogChange({
                      kind: "policy-mode",
                      policyId: policy.id,
                      to: policyEnforcing ? "audit" : "enforce",
                    });
                  }}
                  variant="primary"
                >
                  {intl.formatMessage(
                    policyEnforcing
                      ? messages.returnToAudit
                      : messages.startEnforcing,
                  )}
                </Button>
              </ActionListItem>
              <ActionListItem>
                <Button
                  component="a"
                  variant="secondary"
                  {...navigationLinkProps(
                    links.editHref(policy.id),
                    links.onNavigate,
                  )}
                >
                  {intl.formatMessage(messages.edit)}
                </Button>
              </ActionListItem>
              <ActionListItem>
                <Button
                  isAriaDisabled={policy.builtIn}
                  isDanger
                  onClick={() => {
                    onDialogChange({ kind: "delete", policyId: policy.id });
                  }}
                  variant="secondary"
                >
                  {intl.formatMessage(messages.delete)}
                </Button>
              </ActionListItem>
            </ActionList>
          </FlexItem>
        </Flex>
      </PageSection>
      <PageSection
        data-page-region="body"
        hasBodyWrapper={false}
        isFilled
        variant="default"
      >
        <Stack hasGutter>
          {writeFailure ? (
            <StackItem>
              <PlatformPolicyWriteAlert
                failure={writeFailure}
                onDismiss={onDismissWriteFailure}
              />
            </StackItem>
          ) : null}
          <StackItem>
            <Tabs
              activeKey={activeTab}
              aria-label={intl.formatMessage(messages.detailTabsLabel)}
              mountOnEnter
              onSelect={(_event, key) => {
                onTabChange(toPlatformPolicyDetailTab(String(key)));
              }}
              unmountOnExit
            >
              <Tab
                eventKey="overview"
                title={
                  <TabTitleText>
                    {intl.formatMessage(messages.tabOverview)}
                  </TabTitleText>
                }
              >
                <OverviewTab globalMode={globalMode} policy={policy} />
              </Tab>
              <Tab
                eventKey="gateways"
                title={
                  <TabTitleText>
                    {intl.formatMessage(messages.tabGateways)}
                  </TabTitleText>
                }
              >
                <GatewaysTab
                  globalMode={globalMode}
                  links={links}
                  policy={policy}
                />
              </Tab>
              <Tab
                eventKey="findings"
                title={
                  <TabTitleText>
                    {intl.formatMessage(messages.tabFindings, {
                      count: findings.total,
                    })}
                  </TabTitleText>
                }
              >
                <FindingsTab
                  filter={findingsFilter}
                  findings={findings}
                  onFilterChange={onFindingsFilterChange}
                  onPageChange={onFindingsPageChange}
                  policy={policy}
                />
              </Tab>
            </Tabs>
          </StackItem>
        </Stack>
      </PageSection>
      <PlatformPolicyDialogs
        dialog={dialog}
        isWriting={isWriting}
        onClose={() => {
          onDialogChange(undefined);
        }}
        onConfirm={onConfirmDialog}
        policies={[policy]}
      />
    </>
  );
}
