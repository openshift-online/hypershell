import {
  Alert,
  Button,
  Card,
  CardBody,
  CardFooter,
  CardHeader,
  CardTitle,
  ClipboardCopy,
  CodeBlock,
  CodeBlockCode,
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
  EmptyStateBody,
  ExpandableSection,
  Flex,
  FlexItem,
  HelperText,
  HelperTextItem,
  Label,
  LabelGroup,
  List,
  ListItem,
  Stack,
  StackItem,
  Title,
  ToggleGroup,
  ToggleGroupItem,
  Toolbar,
  ToolbarContent,
  ToolbarItem,
} from "@patternfly/react-core";
import { Table, Tbody, Td, Th, Thead, Tr } from "@patternfly/react-table";
import {
  ExternalLinkAltIcon,
  SearchIcon,
  SyncAltIcon,
} from "@patternfly/react-icons";
import { useState } from "react";

import { HealthStatus } from "../common/health-status";
import {
  EffectiveLimitsTable,
  effectiveLimits,
  findings,
  ModeChain,
  ModeExplanationPopover,
  ModeLabel,
  ModeWithReason,
  OutcomeLabel,
  platformPolicies,
  PolicyRulesTable,
  ruleChips,
  type Policy,
  type PolicyEnforcementMode,
} from "../common/policy-rules";

/*
 * Read-only Policies tab for every access level on a gateway (issue #1145).
 * Gateway owners cannot add policies, set defaults, or change enforcement.
 */

const gatewayName = "payments-gateway";
const currentUser = "ada.lovelace";

function isEnforced(policy: Policy, mode: PolicyEnforcementMode) {
  return mode === "enforce" || policy.mode === "enforce";
}

function modeReason(policy: Policy, mode: PolicyEnforcementMode) {
  if (mode === "enforce") return "By platform enforcement";
  return "Policy setting";
}

function PoliciesTable({
  mode,
  onOpen,
}: {
  mode: PolicyEnforcementMode;
  onOpen: (name: string) => void;
}) {
  return (
    <Table aria-label="Platform policies on this gateway">
      <Thead>
        <Tr>
          <Th width={30}>Policy</Th>
          <Th width={30}>Rules</Th>
          <Th width={25}>Enforcement</Th>
          <Th modifier="wrap" width={15}>
            Would reject, last 24 hours
          </Th>
        </Tr>
      </Thead>
      <Tbody>
        {platformPolicies.map((policy) => {
          const enforced = isEnforced(policy, mode);
          return (
            <Tr key={policy.name}>
              <Td dataLabel="Policy">
                <Button
                  isInline
                  onClick={() => onOpen(policy.name)}
                  variant="link"
                >
                  {policy.name}
                </Button>
              </Td>
              <Td dataLabel="Rules">
                <LabelGroup numLabels={3}>
                  {ruleChips(policy.rules).map((chip) => (
                    <Label isCompact key={chip}>
                      {chip}
                    </Label>
                  ))}
                  {(policy.unrecognized ?? []).map((rule) => (
                    <Label isCompact key={rule} variant="outline">
                      Additional rule
                    </Label>
                  ))}
                </LabelGroup>
              </Td>
              <Td dataLabel="Enforcement">
                <ModeWithReason
                  enforced={enforced}
                  reason={modeReason(policy, mode)}
                />
              </Td>
              <Td dataLabel="Would reject, last 24 hours">
                {enforced ? "-" : 2}
              </Td>
            </Tr>
          );
        })}
      </Tbody>
    </Table>
  );
}

function FindingsCard({ allEnforced }: { allEnforced: boolean }) {
  const [scope, setScope] = useState<"mine" | "all">("all");
  const [outcome, setOutcome] = useState<"all" | "reject" | "default">("all");
  const rows = findings.filter(
    (finding) =>
      (scope === "all" || finding.user === currentUser) &&
      (outcome === "all" || finding.outcome === outcome),
  );
  const rejectCount = findings.filter(
    ({ outcome: value }) => value === "reject",
  ).length;

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
                Audit findings, last 24 hours
              </Title>
            </FlexItem>
            {!allEnforced && rejectCount > 0 ? (
              <FlexItem>
                <HealthStatus
                  appearance="warning"
                  label={`${rejectCount} operations would be rejected`}
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
            titleText="No audit-only policies"
            variant="sm"
          >
            <EmptyStateBody>
              Every policy on this gateway is enforced. OpenShell rejects
              operations that break a rule, so there is nothing to audit.
            </EmptyStateBody>
          </EmptyState>
        ) : (
          <Stack hasGutter>
            <StackItem>
              <Content component="p">
                Audit-only policies allow these operations. They would be
                rejected or changed when the policy is enforced.
              </Content>
            </StackItem>
            <StackItem>
              <Toolbar>
                <ToolbarContent>
                  <ToolbarItem>
                    <ToggleGroup aria-label="Whose operations">
                      <ToggleGroupItem
                        buttonId="findings-mine"
                        isSelected={scope === "mine"}
                        onChange={() => setScope("mine")}
                        text="My operations"
                      />
                      <ToggleGroupItem
                        buttonId="findings-all"
                        isSelected={scope === "all"}
                        onChange={() => setScope("all")}
                        text="All operations"
                      />
                    </ToggleGroup>
                  </ToolbarItem>
                  <ToolbarItem>
                    <ToggleGroup aria-label="Outcome">
                      <ToggleGroupItem
                        buttonId="findings-outcome-all"
                        isSelected={outcome === "all"}
                        onChange={() => setOutcome("all")}
                        text="All outcomes"
                      />
                      <ToggleGroupItem
                        buttonId="findings-outcome-reject"
                        isSelected={outcome === "reject"}
                        onChange={() => setOutcome("reject")}
                        text="Would reject"
                      />
                      <ToggleGroupItem
                        buttonId="findings-outcome-default"
                        isSelected={outcome === "default"}
                        onChange={() => setOutcome("default")}
                        text="Would add defaults"
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
                  titleText="No findings"
                  variant="xs"
                >
                  <EmptyStateBody>
                    No operations in the last 24 hours match these filters.
                  </EmptyStateBody>
                </EmptyState>
              ) : (
                <Table aria-label="Audit findings">
                  <Thead>
                    <Tr>
                      <Th width={20}>Time</Th>
                      <Th width={15}>User</Th>
                      <Th width={15}>Operation</Th>
                      <Th width={15}>Outcome</Th>
                      <Th width={35}>Detail</Th>
                    </Tr>
                  </Thead>
                  <Tbody>
                    {rows.map((finding) => (
                      <Tr key={`${finding.time}-${finding.user}`}>
                        <Td dataLabel="Time">{finding.time}</Td>
                        <Td dataLabel="User">{finding.user}</Td>
                        <Td dataLabel="Operation">{finding.operation}</Td>
                        <Td dataLabel="Outcome">
                          <OutcomeLabel outcome={finding.outcome} />
                        </Td>
                        <Td dataLabel="Detail">
                          {finding.detail} ({finding.policy})
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
  mode,
  policy,
  onClose,
}: {
  mode: PolicyEnforcementMode;
  policy: Policy;
  onClose: () => void;
}) {
  return (
    <DrawerPanelContent defaultSize="520px" isResizable>
      <DrawerHead>
        <Stack>
          <StackItem>
            <Content component="small">Platform policy</Content>
          </StackItem>
          <StackItem>
            <Title headingLevel="h3" size="xl">
              {policy.name}
            </Title>
          </StackItem>
        </Stack>
        <DrawerActions>
          <DrawerCloseButton onClick={onClose} />
        </DrawerActions>
      </DrawerHead>
      <DrawerPanelBody>
        <Stack hasGutter>
          <StackItem>
            <Title headingLevel="h4" size="md">
              Message for users
            </Title>
            <Content component="blockquote">{policy.userMessage}</Content>
            {policy.documentationUrl ? (
              <Button
                component="a"
                href={policy.documentationUrl}
                icon={<ExternalLinkAltIcon />}
                iconPosition="end"
                isInline
                target="_blank"
                variant="link"
              >
                Policy documentation
              </Button>
            ) : null}
          </StackItem>
          <StackItem>
            <Title headingLevel="h4" size="md">
              Rules
            </Title>
            {ruleChips(policy.rules).length ? (
              <PolicyRulesTable name={policy.name} rules={policy.rules} />
            ) : null}
            {policy.unrecognized?.length ? (
              <List>
                {policy.unrecognized.map((rule) => (
                  <ListItem key={rule}>{rule}</ListItem>
                ))}
              </List>
            ) : null}
          </StackItem>
          <StackItem>
            <ExpandableSection toggleText="Show Cedar source">
              <CodeBlock>
                <CodeBlockCode>{policy.cedar}</CodeBlockCode>
              </CodeBlock>
            </ExpandableSection>
          </StackItem>
          <StackItem>
            <Title headingLevel="h4" size="md">
              Why it is {isEnforced(policy, mode) ? "enforced" : "audit only"}
            </Title>
            <ModeChain platformEnforcement={mode} policyMode={policy.mode} />
          </StackItem>
          <StackItem>
            <DescriptionList isCompact isHorizontal>
              <DescriptionListGroup>
                <DescriptionListTerm>Last updated</DescriptionListTerm>
                <DescriptionListDescription>
                  {policy.updated} by {policy.updatedBy}
                </DescriptionListDescription>
              </DescriptionListGroup>
            </DescriptionList>
          </StackItem>
        </Stack>
      </DrawerPanelBody>
    </DrawerPanelContent>
  );
}

export function GatewayPoliciesTab({
  drawerPolicy,
  mode = "audit",
}: {
  drawerPolicy?: string;
  mode?: PolicyEnforcementMode;
}) {
  const [openPolicy, setOpenPolicy] = useState(drawerPolicy);
  const selected = platformPolicies.find(({ name }) => name === openPolicy);
  const allEnforced = platformPolicies.every((policy) =>
    isEnforced(policy, mode),
  );
  const limits = effectiveLimits(mode);
  const additionalRules = platformPolicies.flatMap((policy) =>
    (policy.unrecognized ?? []).map((rule) => ({ policy, rule })),
  );

  const content = (
    <Stack hasGutter>
      <StackItem>
        <Flex justifyContent={{ default: "justifyContentSpaceBetween" }}>
          <FlexItem>
            <Title headingLevel="h2" size="xl">
              Policies
            </Title>
            <Content component="p">
              Platform administrators set these rules for sandboxes on this
              gateway.
            </Content>
          </FlexItem>
          <FlexItem>
            <Flex
              alignItems={{ default: "alignItemsCenter" }}
              gap={{ default: "gapSm" }}
            >
              <FlexItem>
                <Content component="small">Platform enforcement</Content>
              </FlexItem>
              <FlexItem>
                <ModeLabel enforced={mode === "enforce"} />
              </FlexItem>
              <FlexItem>
                <ModeExplanationPopover platformEnforcement={mode} />
              </FlexItem>
              <FlexItem>
                <Button
                  aria-label="Refresh policies"
                  icon={<SyncAltIcon />}
                  title="Refresh policies"
                  variant="plain"
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
                Sandbox limits on this gateway
              </Title>
            </CardTitle>
            <Content component="p">
              {mode === "enforce"
                ? "OpenShell rejects sandbox operations outside these limits and adds the fleet default when a value is missing."
                : "OpenShell rejects sandbox operations outside enforced limits. Values marked Audit only are recorded but not rejected yet. Missing values receive the fleet default."}
            </Content>
          </CardHeader>
          <CardBody>
            <Stack hasGutter>
              <StackItem>
                <EffectiveLimitsTable rows={limits} />
              </StackItem>
              {additionalRules.length ? (
                <StackItem>
                  <Alert
                    isInline
                    isPlain
                    title={`${additionalRules.length} additional rule applies`}
                    variant="info"
                  >
                    <List>
                      {additionalRules.map(({ policy, rule }) => (
                        <ListItem key={rule}>
                          {rule} (
                          <Button
                            isInline
                            onClick={() => setOpenPolicy(policy.name)}
                            variant="link"
                          >
                            {policy.name}
                          </Button>
                          )
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
                  Create a sandbox that fits
                </Title>
              </StackItem>
              <StackItem>
                <ClipboardCopy
                  clickTip="Copied"
                  hoverTip="Copy"
                  isCode
                  isReadOnly
                >
                  {`openshell sandbox create -g ${gatewayName} --name my-sandbox --cpu 2 --memory 4Gi`}
                </ClipboardCopy>
              </StackItem>
              <StackItem>
                <HelperText>
                  <HelperTextItem>
                    OpenShell adds the fleet default requests: 500m CPU and 1Gi
                    memory.
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
                Platform policies ({platformPolicies.length})
              </Title>
            </CardTitle>
          </CardHeader>
          <CardBody>
            <PoliciesTable mode={mode} onOpen={setOpenPolicy} />
          </CardBody>
        </Card>
      </StackItem>
      <StackItem>
        <FindingsCard allEnforced={allEnforced} />
      </StackItem>
    </Stack>
  );

  return (
    <Drawer isExpanded={selected !== undefined} position="end">
      <DrawerContent
        panelContent={
          selected ? (
            <PolicyDrawerPanel
              mode={mode}
              onClose={() => setOpenPolicy(undefined)}
              policy={selected}
            />
          ) : undefined
        }
      >
        <DrawerContentBody>{content}</DrawerContentBody>
      </DrawerContent>
    </Drawer>
  );
}
