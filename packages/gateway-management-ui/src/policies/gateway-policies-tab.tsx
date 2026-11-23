import {
  Alert,
  Button,
  Card,
  CardBody,
  CardFooter,
  CardHeader,
  CardTitle,
  ClipboardCopy,
  Content,
  DescriptionList,
  DescriptionListDescription,
  DescriptionListGroup,
  DescriptionListTerm,
  Drawer,
  DrawerActions,
  DrawerCloseButton,
  DrawerContent,
  DrawerContentBody,
  DrawerHead,
  DrawerPanelBody,
  DrawerPanelContent,
  EmptyState,
  EmptyStateActions,
  EmptyStateBody,
  EmptyStateFooter,
  EmptyStateVariant,
  ExpandableSection,
  Flex,
  FlexItem,
  HelperText,
  HelperTextItem,
  Label,
  LabelGroup,
  List,
  ListItem,
  Spinner,
  Stack,
  StackItem,
  Title,
  ToggleGroup,
  ToggleGroupItem,
  Toolbar,
  ToolbarContent,
  ToolbarItem,
} from "@patternfly/react-core";
import {
  ExternalLinkAltIcon,
  SearchIcon,
  ShieldAltIcon,
} from "@patternfly/react-icons";
import { Table, Tbody, Td, Th, Thead, Tr } from "@patternfly/react-table";
import { useQuery } from "@tanstack/react-query";
import { useState, type ReactElement } from "react";
import { FormattedMessage, useIntl, type IntlShape } from "react-intl";

import type {
  EffectivePolicy,
  GatewayEffectivePolicies,
  PolicyFinding,
  PolicyFindingOutcome,
  PolicyFleetDefaults,
  PolicyRenderedRule,
  PolicyResourceKey,
} from "../application/policy-types";
import { useGatewayUi } from "../gateway-ui-provider";
import { GatewayStatus } from "../gateways/gateway-status";
import { ResourceRefreshButton } from "../shared/resource-refresh-button";
import { gatewayPoliciesMessages as messages } from "./gateway-policies-messages";
import {
  gatewayEffectivePoliciesQueryKey,
  gatewayEffectivePoliciesStaleMilliseconds,
} from "./policy-data";
import { policyMessages } from "./policy-messages";
import {
  formatPolicyTime,
  PolicyCedarRule,
  policyFindingDetail,
  PolicyModeChain,
  PolicyModeExplanationPopover,
  PolicyModeLabel,
  policyModeReasonText,
  PolicyModeWithReason,
  policyOperationText,
  PolicyOutcomeLabel,
  policyResourceLabel,
  policyRuleResources,
  PolicyRulesTable,
  PolicySourceLabel,
} from "./policy-presentation";
import {
  additionalRules,
  fleetDefaultFor,
  policyResourceFields,
  rowBounds,
  type PolicyBoundSide,
} from "./policy-quantities";

/** Resources a compliant `openshell sandbox create` command sets. */
export interface SandboxResources {
  cpu?: string;
  memory?: string;
}

/**
 * The `--cpu` and `--memory` values a sandbox command uses: the fleet default
 * limits, never hard-coded values (WEB-GLIS-26.02).
 */
export function sandboxResourcesFromDefaults(
  defaults: PolicyFleetDefaults,
): SandboxResources {
  return {
    ...(defaults.cpuLimit ? { cpu: defaults.cpuLimit } : {}),
    ...(defaults.memoryLimit ? { memory: defaults.memoryLimit } : {}),
  };
}

/** A copyable `openshell sandbox create` command that fits the limits. */
export function buildFittingSandboxCommand(
  gatewayName: string,
  resources: SandboxResources,
): string {
  return [
    `openshell sandbox create -g ${gatewayName} --name my-sandbox`,
    ...(resources.cpu ? [`--cpu ${resources.cpu}`] : []),
    ...(resources.memory ? [`--memory ${resources.memory}`] : []),
  ].join(" ");
}

type FindingScope = "all" | "mine";
type FindingOutcomeFilter = "all" | PolicyFindingOutcome;

/** One attributed value in a limits cell. */
interface LimitEntry {
  enforced: boolean;
  isFleetDefault: boolean;
  source: string;
  value: string;
}

interface LimitRow {
  default: readonly LimitEntry[];
  maximum: readonly LimitEntry[];
  minimum: readonly LimitEntry[];
  resource: PolicyResourceKey;
}

function limitRows(
  intl: IntlShape,
  data: GatewayEffectivePolicies,
): readonly LimitRow[] {
  const attributed = data.policies.flatMap((policy) =>
    rowBounds(policy.rendered).map((bound) => ({ bound, policy })),
  );
  const entriesFor = (resource: PolicyResourceKey, side: PolicyBoundSide) =>
    attributed
      .filter(({ bound }) => bound.resource === resource && bound.side === side)
      .map(({ bound, policy }) => ({
        enforced: policy.effectiveMode === "enforce",
        isFleetDefault: false,
        source: policy.name,
        value: bound.bound.display,
      }));
  return policyResourceFields.flatMap(([resource]) => {
    const fleetDefault = fleetDefaultFor(data.defaults, resource);
    const row: LimitRow = {
      default: fleetDefault
        ? [
            {
              enforced: true,
              isFleetDefault: true,
              source: intl.formatMessage(policyMessages.fleetDefault),
              value: fleetDefault,
            },
          ]
        : [],
      maximum: entriesFor(resource, "maximum"),
      minimum: entriesFor(resource, "minimum"),
      resource,
    };
    return row.default.length || row.maximum.length || row.minimum.length
      ? [row]
      : [];
  });
}

function LimitCell({
  entries,
}: {
  entries: readonly LimitEntry[];
}): ReactElement {
  const intl = useIntl();
  if (!entries.length) {
    return (
      <Content component="small">
        {intl.formatMessage(policyMessages.noRule)}
      </Content>
    );
  }
  return (
    <Stack hasGutter>
      {entries.map((entry) => (
        <StackItem key={`${entry.source}-${entry.value}`}>
          <Stack>
            <StackItem>
              <Content component="p">
                <strong>{entry.value}</strong>
              </Content>
            </StackItem>
            <StackItem>
              <Flex flexWrap={{ default: "wrap" }} gap={{ default: "gapXs" }}>
                <FlexItem>
                  <PolicySourceLabel
                    isFleetDefault={entry.isFleetDefault}
                    name={entry.source}
                  />
                </FlexItem>
                {entry.enforced ? null : (
                  <FlexItem>
                    <PolicyModeLabel enforced={false} isCompact />
                  </FlexItem>
                )}
              </Flex>
            </StackItem>
          </Stack>
        </StackItem>
      ))}
    </Stack>
  );
}

function LimitsTable({ rows }: { rows: readonly LimitRow[] }): ReactElement {
  const intl = useIntl();
  const resource = intl.formatMessage(policyMessages.columnResource);
  const minimum = intl.formatMessage(policyMessages.columnMinimum);
  const fallback = intl.formatMessage(policyMessages.columnDefault);
  const maximum = intl.formatMessage(policyMessages.columnMaximum);
  return (
    <Table aria-label={intl.formatMessage(messages.limitsTableLabel)}>
      <Thead>
        <Tr>
          <Th width={20}>{resource}</Th>
          <Th width={25}>{minimum}</Th>
          <Th width={30}>{fallback}</Th>
          <Th width={25}>{maximum}</Th>
        </Tr>
      </Thead>
      <Tbody>
        {rows.map((row) => (
          <Tr key={row.resource}>
            <Th dataLabel={resource}>
              {policyResourceLabel(intl, row.resource)}
            </Th>
            <Td dataLabel={minimum}>
              <LimitCell entries={row.minimum} />
            </Td>
            <Td dataLabel={fallback}>
              <LimitCell entries={row.default} />
            </Td>
            <Td dataLabel={maximum}>
              <LimitCell entries={row.maximum} />
            </Td>
          </Tr>
        ))}
      </Tbody>
    </Table>
  );
}

function wouldRejectCount(
  policy: EffectivePolicy,
  findings: readonly PolicyFinding[],
): number {
  return findings.filter(
    (finding) =>
      finding.outcome === "would-reject" &&
      finding.policyNames.includes(policy.name),
  ).length;
}

function PoliciesTable({
  findings,
  onOpen,
  policies,
}: {
  findings: readonly PolicyFinding[];
  onOpen: (name: string) => void;
  policies: readonly EffectivePolicy[];
}): ReactElement {
  const intl = useIntl();
  const policyColumn = intl.formatMessage(messages.columnPolicy);
  const rulesColumn = intl.formatMessage(messages.columnRules);
  const enforcementColumn = intl.formatMessage(messages.columnEnforcement);
  const rejectColumn = intl.formatMessage(messages.columnWouldReject);
  return (
    <Table aria-label={intl.formatMessage(messages.policiesTableLabel)}>
      <Thead>
        <Tr>
          <Th width={30}>{policyColumn}</Th>
          <Th width={30}>{rulesColumn}</Th>
          <Th width={25}>{enforcementColumn}</Th>
          <Th modifier="wrap" width={15}>
            {rejectColumn}
          </Th>
        </Tr>
      </Thead>
      <Tbody>
        {policies.map((policy) => {
          const enforced = policy.effectiveMode === "enforce";
          const extra = additionalRules(policy.rendered);
          return (
            <Tr key={policy.id || policy.name}>
              <Td dataLabel={policyColumn}>
                <Button
                  isInline
                  onClick={() => {
                    onOpen(policy.name);
                  }}
                  variant="link"
                >
                  {policy.name}
                </Button>
              </Td>
              <Td dataLabel={rulesColumn}>
                <LabelGroup numLabels={3}>
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
              </Td>
              <Td dataLabel={enforcementColumn}>
                <PolicyModeWithReason
                  enforced={enforced}
                  reason={policyModeReasonText(
                    intl,
                    policy.effectiveModeReason,
                  )}
                />
              </Td>
              <Td dataLabel={rejectColumn}>
                {enforced
                  ? intl.formatMessage(messages.notApplicable)
                  : wouldRejectCount(policy, findings)}
              </Td>
            </Tr>
          );
        })}
      </Tbody>
    </Table>
  );
}

function FindingsCard({
  allEnforced,
  data,
}: {
  allEnforced: boolean;
  data: GatewayEffectivePolicies;
}): ReactElement {
  const intl = useIntl();
  const { currentUser } = useGatewayUi();
  const [scope, setScope] = useState<FindingScope>("all");
  const [outcome, setOutcome] = useState<FindingOutcomeFilter>("all");
  const ownIdentities = new Set(
    [currentUser?.subject, currentUser?.username].filter(
      (value): value is string => Boolean(value),
    ),
  );
  const rows = data.findings.filter(
    (finding) =>
      (scope === "all" || ownIdentities.has(finding.principalSubject)) &&
      (outcome === "all" || finding.outcome === outcome),
  );
  const rejectCount = data.findingsLast24h.wouldReject;
  const timeColumn = intl.formatMessage(messages.columnTime);
  const userColumn = intl.formatMessage(messages.columnUser);
  const operationColumn = intl.formatMessage(messages.columnOperation);
  const outcomeColumn = intl.formatMessage(messages.columnOutcome);
  const detailColumn = intl.formatMessage(messages.columnDetail);

  return (
    <Card>
      <CardHeader>
        <CardTitle>
          <Flex
            alignItems={{ default: "alignItemsCenter" }}
            gap={{ default: "gapSm" }}
          >
            <FlexItem>
              <Title headingLevel="h3" size="lg">
                {intl.formatMessage(messages.findingsTitle)}
              </Title>
            </FlexItem>
            {!allEnforced && rejectCount > 0 ? (
              <FlexItem>
                <GatewayStatus
                  label={intl.formatMessage(messages.findingsWouldReject, {
                    count: rejectCount,
                  })}
                  status="warning"
                />
              </FlexItem>
            ) : null}
          </Flex>
        </CardTitle>
      </CardHeader>
      <CardBody>
        {allEnforced ? (
          <EmptyState
            headingLevel="h4"
            icon={SearchIcon}
            titleText={intl.formatMessage(messages.allEnforcedTitle)}
            variant={EmptyStateVariant.sm}
          >
            <EmptyStateBody>
              {intl.formatMessage(messages.allEnforcedBody)}
            </EmptyStateBody>
          </EmptyState>
        ) : (
          <Stack hasGutter>
            <StackItem>
              <Content component="p">
                {intl.formatMessage(messages.findingsIntro)}
              </Content>
            </StackItem>
            <StackItem>
              <Toolbar>
                <ToolbarContent>
                  <ToolbarItem>
                    <ToggleGroup
                      aria-label={intl.formatMessage(messages.whoseOperations)}
                    >
                      <ToggleGroupItem
                        buttonId="policy-findings-mine"
                        isSelected={scope === "mine"}
                        onChange={() => {
                          setScope("mine");
                        }}
                        text={intl.formatMessage(messages.myOperations)}
                      />
                      <ToggleGroupItem
                        buttonId="policy-findings-all"
                        isSelected={scope === "all"}
                        onChange={() => {
                          setScope("all");
                        }}
                        text={intl.formatMessage(messages.allOperations)}
                      />
                    </ToggleGroup>
                  </ToolbarItem>
                  <ToolbarItem>
                    <ToggleGroup
                      aria-label={intl.formatMessage(messages.outcomeFilter)}
                    >
                      <ToggleGroupItem
                        buttonId="policy-findings-outcome-all"
                        isSelected={outcome === "all"}
                        onChange={() => {
                          setOutcome("all");
                        }}
                        text={intl.formatMessage(messages.allOutcomes)}
                      />
                      <ToggleGroupItem
                        buttonId="policy-findings-outcome-reject"
                        isSelected={outcome === "would-reject"}
                        onChange={() => {
                          setOutcome("would-reject");
                        }}
                        text={intl.formatMessage(
                          policyMessages.outcomeWouldReject,
                        )}
                      />
                      <ToggleGroupItem
                        buttonId="policy-findings-outcome-default"
                        isSelected={outcome === "would-add-defaults"}
                        onChange={() => {
                          setOutcome("would-add-defaults");
                        }}
                        text={intl.formatMessage(
                          policyMessages.outcomeWouldAddDefaults,
                        )}
                      />
                    </ToggleGroup>
                  </ToolbarItem>
                </ToolbarContent>
              </Toolbar>
            </StackItem>
            <StackItem>
              {rows.length === 0 ? (
                <EmptyState
                  headingLevel="h4"
                  titleText={intl.formatMessage(messages.noFindingsTitle)}
                  variant={EmptyStateVariant.xs}
                >
                  <EmptyStateBody>
                    {intl.formatMessage(messages.noFindingsBody)}
                  </EmptyStateBody>
                </EmptyState>
              ) : (
                <Table
                  aria-label={intl.formatMessage(messages.findingsTableLabel)}
                >
                  <Thead>
                    <Tr>
                      <Th width={20}>{timeColumn}</Th>
                      <Th width={15}>{userColumn}</Th>
                      <Th width={15}>{operationColumn}</Th>
                      <Th width={15}>{outcomeColumn}</Th>
                      <Th width={35}>{detailColumn}</Th>
                    </Tr>
                  </Thead>
                  <Tbody>
                    {rows.map((finding) => (
                      <Tr key={finding.id}>
                        <Td dataLabel={timeColumn}>
                          {formatPolicyTime(intl, finding.occurredAt)}
                        </Td>
                        <Td dataLabel={userColumn}>
                          {finding.principalSubject}
                        </Td>
                        <Td dataLabel={operationColumn}>
                          {policyOperationText(intl, finding.operation)}
                        </Td>
                        <Td dataLabel={outcomeColumn}>
                          <PolicyOutcomeLabel outcome={finding.outcome} />
                        </Td>
                        <Td dataLabel={detailColumn}>
                          {policyFindingDetail(intl, finding)}
                        </Td>
                      </Tr>
                    ))}
                  </Tbody>
                </Table>
              )}
            </StackItem>
          </Stack>
        )}
      </CardBody>
    </Card>
  );
}

function PolicyDrawerPanel({
  data,
  onClose,
  policy,
}: {
  data: GatewayEffectivePolicies;
  onClose: () => void;
  policy: EffectivePolicy;
}): ReactElement {
  const intl = useIntl();
  const [isCedarExpanded, setIsCedarExpanded] = useState(false);
  const extra = additionalRules(policy.rendered);
  return (
    <DrawerPanelContent defaultSize="520px" isResizable>
      <DrawerHead>
        <Stack>
          <StackItem>
            <Content component="small">
              {intl.formatMessage(messages.drawerKind)}
            </Content>
          </StackItem>
          <StackItem>
            <Title headingLevel="h3" size="xl">
              {policy.name}
            </Title>
          </StackItem>
        </Stack>
        <DrawerActions>
          <DrawerCloseButton
            aria-label={intl.formatMessage(messages.closeDrawer)}
            onClick={onClose}
          />
        </DrawerActions>
      </DrawerHead>
      <DrawerPanelBody>
        <Stack hasGutter>
          <StackItem>
            <Title headingLevel="h4" size="md">
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
          <StackItem>
            <Title headingLevel="h4" size="md">
              {intl.formatMessage(messages.drawerRules)}
            </Title>
            {policyRuleResources(policy.rendered).length ? (
              <PolicyRulesTable name={policy.name} rules={policy.rendered} />
            ) : null}
            {extra.length ? (
              <List>
                {extra.map((rule, index) => (
                  <ListItem key={`${String(index)}-${rule.cedar}`}>
                    {rule.summary.length
                      ? intl.formatList(rule.summary)
                      : intl.formatMessage(policyMessages.shownAsCedar)}
                  </ListItem>
                ))}
              </List>
            ) : null}
          </StackItem>
          <StackItem>
            <ExpandableSection
              isExpanded={isCedarExpanded}
              onToggle={(_event, expanded) => {
                setIsCedarExpanded(expanded);
              }}
              toggleText={intl.formatMessage(
                isCedarExpanded
                  ? policyMessages.hideCedarSource
                  : policyMessages.showCedarSource,
              )}
            >
              <PolicyCedarRule
                rule={{
                  bounds: [],
                  cedar: policy.cedar,
                  recognized: false,
                  summary: [],
                }}
              />
            </ExpandableSection>
          </StackItem>
          <StackItem>
            <Title headingLevel="h4" size="md">
              {intl.formatMessage(messages.drawerWhy, {
                mode: policy.effectiveMode,
              })}
            </Title>
            <PolicyModeChain
              effectiveMode={policy.effectiveMode}
              globalMode={data.globalMode}
              policyMode={policy.enforcementMode}
            />
          </StackItem>
          {policy.updatedAt ? (
            <StackItem>
              <DescriptionList isCompact isHorizontal>
                <DescriptionListGroup>
                  <DescriptionListTerm>
                    {intl.formatMessage(messages.lastUpdated)}
                  </DescriptionListTerm>
                  <DescriptionListDescription>
                    {formatPolicyTime(intl, policy.updatedAt)}
                  </DescriptionListDescription>
                </DescriptionListGroup>
              </DescriptionList>
            </StackItem>
          ) : null}
        </Stack>
      </DrawerPanelBody>
    </DrawerPanelContent>
  );
}

interface AdditionalRuleEntry {
  policy: EffectivePolicy;
  rule: PolicyRenderedRule;
}

function PoliciesContent({
  data,
  gatewayName,
  isRefreshing,
  onOpen,
  onRefresh,
}: {
  data: GatewayEffectivePolicies;
  gatewayName: string;
  isRefreshing: boolean;
  onOpen: (name: string) => void;
  onRefresh: () => void;
}): ReactElement {
  const intl = useIntl();
  const enforced = data.globalMode === "enforce";
  const allEnforced = data.policies.every(
    (policy) => policy.effectiveMode === "enforce",
  );
  const rows = limitRows(intl, data);
  const extraRules: readonly AdditionalRuleEntry[] = data.policies.flatMap(
    (policy) =>
      additionalRules(policy.rendered).map((rule) => ({ policy, rule })),
  );
  const resources = sandboxResourcesFromDefaults(data.defaults);
  const { cpuRequest, memoryRequest } = data.defaults;

  return (
    <Stack hasGutter>
      <StackItem>
        <Flex justifyContent={{ default: "justifyContentSpaceBetween" }}>
          <FlexItem>
            <Title headingLevel="h2" size="xl">
              {intl.formatMessage(messages.tab)}
            </Title>
            <Content component="p">
              {intl.formatMessage(messages.description)}
            </Content>
          </FlexItem>
          <FlexItem>
            <Flex
              alignItems={{ default: "alignItemsCenter" }}
              gap={{ default: "gapSm" }}
            >
              <FlexItem>
                <Content component="small">
                  {intl.formatMessage(policyMessages.platformEnforcement)}
                </Content>
              </FlexItem>
              <FlexItem>
                <PolicyModeLabel enforced={enforced} />
              </FlexItem>
              <FlexItem>
                <PolicyModeExplanationPopover globalMode={data.globalMode} />
              </FlexItem>
              <FlexItem>
                <ResourceRefreshButton
                  ariaLabel={intl.formatMessage(messages.refresh)}
                  isRefreshing={isRefreshing}
                  onRefresh={onRefresh}
                />
              </FlexItem>
            </Flex>
          </FlexItem>
        </Flex>
      </StackItem>
      <StackItem>
        <Card>
          <CardHeader>
            <CardTitle>
              <Title headingLevel="h3" size="lg">
                {intl.formatMessage(messages.limitsTitle)}
              </Title>
            </CardTitle>
            <Content component="p">
              {intl.formatMessage(
                enforced ? messages.limitsEnforced : messages.limitsAudit,
              )}
            </Content>
          </CardHeader>
          <CardBody>
            <Stack hasGutter>
              <StackItem>
                <LimitsTable rows={rows} />
              </StackItem>
              {extraRules.length ? (
                <StackItem>
                  <Alert
                    isInline
                    isPlain
                    title={intl.formatMessage(messages.additionalRulesTitle, {
                      count: extraRules.length,
                    })}
                    variant="info"
                  >
                    <List>
                      {extraRules.map(({ policy, rule }, index) => (
                        <ListItem key={`${policy.name}-${String(index)}`}>
                          <Stack hasGutter>
                            <StackItem>
                              <FormattedMessage
                                {...messages.additionalRuleItem}
                                values={{
                                  message: policy.userMessage,
                                  policy: (
                                    <Button
                                      isInline
                                      onClick={() => {
                                        onOpen(policy.name);
                                      }}
                                      variant="link"
                                    >
                                      {policy.name}
                                    </Button>
                                  ),
                                }}
                              />
                            </StackItem>
                            <StackItem>
                              <PolicyCedarRule rule={rule} />
                            </StackItem>
                          </Stack>
                        </ListItem>
                      ))}
                    </List>
                  </Alert>
                </StackItem>
              ) : null}
            </Stack>
          </CardBody>
          <CardFooter>
            <Stack hasGutter>
              <StackItem>
                <Title headingLevel="h4" size="md">
                  {intl.formatMessage(messages.createFitsTitle)}
                </Title>
              </StackItem>
              <StackItem>
                <ClipboardCopy
                  clickTip={intl.formatMessage(messages.copied)}
                  hoverTip={intl.formatMessage(messages.copy)}
                  isCode
                  isReadOnly
                >
                  {buildFittingSandboxCommand(gatewayName, resources)}
                </ClipboardCopy>
              </StackItem>
              <StackItem>
                <HelperText>
                  <HelperTextItem>
                    {cpuRequest && memoryRequest
                      ? intl.formatMessage(messages.defaultRequestsHelp, {
                          cpu: cpuRequest,
                          memory: memoryRequest,
                        })
                      : intl.formatMessage(messages.defaultRequestsUnknown)}
                  </HelperTextItem>
                </HelperText>
              </StackItem>
            </Stack>
          </CardFooter>
        </Card>
      </StackItem>
      <StackItem>
        <Card>
          <CardHeader>
            <CardTitle>
              <Title headingLevel="h3" size="lg">
                {intl.formatMessage(messages.policiesTitle, {
                  count: data.policies.length,
                })}
              </Title>
            </CardTitle>
          </CardHeader>
          <CardBody>
            {data.policies.length ? (
              <PoliciesTable
                findings={data.findings}
                onOpen={onOpen}
                policies={data.policies}
              />
            ) : (
              <EmptyState
                headingLevel="h4"
                icon={ShieldAltIcon}
                titleText={intl.formatMessage(messages.noPoliciesTitle)}
                variant={EmptyStateVariant.sm}
              >
                <EmptyStateBody>
                  {intl.formatMessage(messages.noPoliciesBody)}
                </EmptyStateBody>
              </EmptyState>
            )}
          </CardBody>
        </Card>
      </StackItem>
      {data.policies.length ? (
        <StackItem>
          <FindingsCard allEnforced={allEnforced} data={data} />
        </StackItem>
      ) : null}
    </Stack>
  );
}

export interface GatewayPoliciesTabProps {
  gatewayId: string;
  gatewayName: string;
  /** Policy whose details drawer is open when the tab first renders. */
  initialPolicyName?: string;
  isActive: boolean;
}

/**
 * Read-only Policies tab for every gateway access level (WEB-GLIS-26). Gateway
 * users never author, edit, or change the mode of a policy here.
 */
export function GatewayPoliciesTab({
  gatewayId,
  gatewayName,
  initialPolicyName,
  isActive,
}: GatewayPoliciesTabProps): ReactElement {
  const intl = useIntl();
  const { gateways } = useGatewayUi();
  const [openPolicyName, setOpenPolicyName] = useState(initialPolicyName);
  const isAvailable = gateways.getGatewayEffectivePolicies !== undefined;
  const policiesQuery = useQuery({
    enabled: isActive && isAvailable && gatewayId.length > 0,
    queryFn: ({ signal }) => {
      if (!gateways.getGatewayEffectivePolicies) {
        throw new Error("effective policies are unavailable");
      }
      return gateways.getGatewayEffectivePolicies(gatewayId, signal);
    },
    queryKey: gatewayEffectivePoliciesQueryKey(gatewayId),
    refetchIntervalInBackground: false,
    retry: false,
    staleTime: gatewayEffectivePoliciesStaleMilliseconds,
  });

  if (!isAvailable) {
    return (
      <EmptyState
        headingLevel="h2"
        titleText={intl.formatMessage(messages.unavailableTitle)}
        variant={EmptyStateVariant.sm}
      >
        <EmptyStateBody>
          {intl.formatMessage(messages.unavailableBody)}
        </EmptyStateBody>
      </EmptyState>
    );
  }
  if (policiesQuery.isPending) {
    return <Spinner aria-label={intl.formatMessage(messages.loading)} />;
  }
  if (policiesQuery.isError) {
    return (
      <EmptyState
        headingLevel="h2"
        status="danger"
        titleText={intl.formatMessage(messages.loadErrorTitle)}
        variant={EmptyStateVariant.sm}
      >
        <EmptyStateBody>
          {intl.formatMessage(messages.loadErrorBody)}
        </EmptyStateBody>
        <EmptyStateFooter>
          <EmptyStateActions>
            <Button
              onClick={() => void policiesQuery.refetch()}
              variant="primary"
            >
              {intl.formatMessage(messages.retry)}
            </Button>
          </EmptyStateActions>
        </EmptyStateFooter>
      </EmptyState>
    );
  }

  const data = policiesQuery.data;
  const selected = data.policies.find(({ name }) => name === openPolicyName);
  return (
    <Drawer isExpanded={selected !== undefined} position="end">
      <DrawerContent
        panelContent={
          selected ? (
            <PolicyDrawerPanel
              data={data}
              onClose={() => {
                setOpenPolicyName(undefined);
              }}
              policy={selected}
            />
          ) : undefined
        }
      >
        <DrawerContentBody>
          <PoliciesContent
            data={data}
            gatewayName={gatewayName}
            isRefreshing={policiesQuery.isFetching}
            onOpen={setOpenPolicyName}
            onRefresh={() => void policiesQuery.refetch()}
          />
        </DrawerContentBody>
      </DrawerContent>
    </Drawer>
  );
}
