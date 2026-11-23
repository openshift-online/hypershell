import "@patternfly/react-core/dist/styles/base.css";

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
  Modal,
  ModalBody,
  ModalFooter,
  ModalHeader,
  ModalVariant,
  PageSection,
  SearchInput,
  Stack,
  StackItem,
  Title,
  Toolbar,
  ToolbarContent,
  ToolbarItem,
} from "@patternfly/react-core";
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
import { ShieldAltIcon } from "@patternfly/react-icons";
import { useState } from "react";

import { HealthStatus } from "../common/health-status";
import {
  ModeExplanationPopover,
  ModeLabel,
  ModeWithReason,
  platformPolicies,
  ruleChips,
  type Policy,
  type PolicyEnforcementMode,
} from "../common/policy-rules";
import { stories, storyLinkProps } from "../common/story-links";

export type PlatformPoliciesState = "default" | "empty";
export type PlatformPoliciesModal =
  | "switch-to-enforced"
  | "switch-to-audit"
  | "start-enforcing"
  | "delete-policy";

const detailLink = () => storyLinkProps(stories.platformPolicyDetail);

function rolloutStatus(policy: Policy) {
  const rollout = policy.rollout ?? [];
  const applied = rollout.filter(({ state }) => state === "applied").length;
  const unsupported = rollout.filter(
    ({ state }) => state === "unsupported",
  ).length;
  const pending = rollout.filter(({ state }) => state === "pending").length;
  const problems = [
    unsupported ? `${unsupported} cannot enforce` : undefined,
    pending ? `${pending} pending` : undefined,
  ].filter(Boolean);
  return (
    <Stack>
      <StackItem>
        <HealthStatus
          appearance={unsupported ? "danger" : pending ? "warning" : "good"}
          label={`Applied on ${applied} of ${rollout.length} gateways`}
        />
      </StackItem>
      {problems.length ? (
        <StackItem>
          <Content component="small">{problems.join(", ")}</Content>
        </StackItem>
      ) : null}
    </Stack>
  );
}

function enforcementCell(policy: Policy, mode: PolicyEnforcementMode) {
  if (mode === "enforce") {
    return <ModeWithReason enforced reason="By platform enforcement" />;
  }
  return (
    <ModeWithReason
      enforced={policy.mode === "enforce"}
      reason="Policy setting"
    />
  );
}

function PoliciesTable({
  mode,
  onModal,
}: {
  mode: PolicyEnforcementMode;
  onModal: (modal: PlatformPoliciesModal) => void;
}) {
  const actions = (policy: Policy): IAction[] => [
    policy.mode === "enforce"
      ? {
          title: "Return to audit only...",
          isAriaDisabled: mode === "enforce",
          description:
            mode === "enforce"
              ? "Platform enforcement is Enforced for every policy."
              : undefined,
          onClick: () => onModal("start-enforcing"),
        }
      : {
          title: "Start enforcing...",
          isAriaDisabled: mode === "enforce",
          description:
            mode === "enforce"
              ? "Platform enforcement already enforces every policy."
              : undefined,
          onClick: () => onModal("start-enforcing"),
        },
    { title: "Edit policy", onClick: () => undefined },
    { isSeparator: true },
    {
      title: "Delete policy",
      isDanger: true,
      onClick: () => onModal("delete-policy"),
    },
  ];

  return (
    <Table aria-label="Platform policies">
      <Thead>
        <Tr>
          <Th width={20}>Policy</Th>
          <Th width={15}>Applies to</Th>
          <Th width={20}>Rules</Th>
          <Th width={15}>Enforcement</Th>
          <Th width={20}>Applied</Th>
          <Th modifier="wrap" width={10}>
            Would reject, last 24 hours
          </Th>
          <Th screenReaderText="Actions" />
        </Tr>
      </Thead>
      <Tbody>
        {platformPolicies.map((policy) => (
          <Tr key={policy.name}>
            <Td dataLabel="Policy">
              <Button component="a" {...detailLink()} isInline variant="link">
                {policy.name}
              </Button>
            </Td>
            <Td dataLabel="Applies to">{policy.appliesTo}</Td>
            <Td dataLabel="Rules">
              <LabelGroup numLabels={2}>
                {ruleChips(policy.rules).map((chip) => (
                  <Label isCompact key={chip}>
                    {chip}
                  </Label>
                ))}
              </LabelGroup>
            </Td>
            <Td dataLabel="Enforcement">{enforcementCell(policy, mode)}</Td>
            <Td dataLabel="Applied">{rolloutStatus(policy)}</Td>
            <Td dataLabel="Would reject, last 24 hours">
              {mode === "enforce" || policy.mode === "enforce"
                ? "-"
                : policy.wouldReject24h}
            </Td>
            <Td isActionCell>
              <ActionsColumn
                items={actions(policy)}
                popperProps={{ position: "right" }}
              />
            </Td>
          </Tr>
        ))}
      </Tbody>
    </Table>
  );
}

function PlatformPoliciesDialog({
  modal,
  onClose,
}: {
  modal: PlatformPoliciesModal;
  onClose: () => void;
}) {
  if (modal === "delete-policy") {
    return (
      <Modal isOpen onClose={onClose} variant={ModalVariant.small}>
        <ModalHeader
          title="Delete sandbox-resource-baseline?"
          titleIconVariant="warning"
        />
        <ModalBody>
          The policy is removed from 4 gateways. This action cannot be undone.
        </ModalBody>
        <ModalFooter>
          <Button variant="danger">Delete policy</Button>
          <Button onClick={onClose} variant="link">
            Cancel
          </Button>
        </ModalFooter>
      </Modal>
    );
  }
  if (modal === "switch-to-audit") {
    return (
      <Modal isOpen onClose={onClose} variant={ModalVariant.small}>
        <ModalHeader
          title="Switch platform enforcement to Audit only?"
          titleIconVariant="warning"
        />
        <ModalBody>
          Policies that are not enforced by their own setting stop rejecting
          operations on 14 gateways. HyperShell records them as audit findings
          instead.
        </ModalBody>
        <ModalFooter>
          <Button variant="primary">Switch to Audit only</Button>
          <Button onClick={onClose} variant="link">
            Cancel
          </Button>
        </ModalFooter>
      </Modal>
    );
  }
  const isGlobal = modal === "switch-to-enforced";
  return (
    <Modal isOpen onClose={onClose} variant={ModalVariant.medium}>
      <ModalHeader
        title={
          isGlobal
            ? "Enforce every policy on all 14 gateways?"
            : "Start enforcing sandbox-resource-baseline?"
        }
        titleIconVariant="warning"
      />
      <ModalBody>
        <Stack hasGutter>
          <StackItem>
            {isGlobal
              ? "OpenShell will reject operations that break any platform or gateway policy and add missing defaults."
              : "OpenShell will reject operations that break this policy on every gateway it applies to, and add its defaults."}
          </StackItem>
          <StackItem>
            <Alert
              isInline
              title="In the last 24 hours, 12 operations would have been rejected"
              variant="warning"
            >
              41 operations would have received default values.
            </Alert>
          </StackItem>
          <StackItem>
            <Table aria-label="Impact by gateway" variant="compact">
              <Thead>
                <Tr>
                  <Th>Gateway</Th>
                  <Th>Would reject</Th>
                  <Th>Note</Th>
                </Tr>
              </Thead>
              <Tbody>
                <Tr>
                  <Td dataLabel="Gateway">ml-training-gateway</Td>
                  <Td dataLabel="Would reject">10</Td>
                  <Td dataLabel="Note">Revision pending</Td>
                </Tr>
                <Tr>
                  <Td dataLabel="Gateway">payments-gateway</Td>
                  <Td dataLabel="Would reject">2</Td>
                  <Td dataLabel="Note">-</Td>
                </Tr>
                <Tr>
                  <Td dataLabel="Gateway">edge-lab-gateway</Td>
                  <Td dataLabel="Would reject">0</Td>
                  <Td dataLabel="Note">
                    Cannot enforce. The VM driver ignores CPU and memory.
                  </Td>
                </Tr>
              </Tbody>
            </Table>
          </StackItem>
        </Stack>
      </ModalBody>
      <ModalFooter>
        <Button variant="primary">
          {isGlobal ? "Enforce on 14 gateways" : "Start enforcing"}
        </Button>
        <Button onClick={onClose} variant="link">
          Cancel
        </Button>
      </ModalFooter>
    </Modal>
  );
}

export function PlatformPoliciesPageSurface({
  mode = "audit",
  modal: initialModal,
  state = "default",
}: {
  mode?: PolicyEnforcementMode;
  modal?: PlatformPoliciesModal;
  state?: PlatformPoliciesState;
}) {
  const [modal, setModal] = useState(initialModal);
  const isEmpty = state === "empty";

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
              Platform policies
            </Title>
            <Content component="p">
              Set sandbox rules for many gateways. Gateway users see these rules
              and their audit findings on each gateway.
            </Content>
          </FlexItem>
          {isEmpty ? null : (
            <FlexItem>
              <Button
                component="a"
                {...storyLinkProps(stories.createPlatformPolicy)}
                variant="primary"
              >
                Create platform policy
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
        <Grid hasGutter>
          {isEmpty ? null : (
            <GridItem>
              <Alert
                actionLinks={
                  <AlertActionLink
                    component="a"
                    {...storyLinkProps(stories.platformPolicyGateways)}
                  >
                    Review rollout
                  </AlertActionLink>
                }
                isInline
                title="sandbox-resource-baseline cannot be enforced on 1 gateway"
                variant="warning"
              >
                edge-lab-gateway uses the VM compute driver, which ignores
                per-sandbox CPU and memory.
              </Alert>
            </GridItem>
          )}
          <GridItem lg={6}>
            <Card isFullHeight>
              <CardHeader>
                <CardTitle>
                  <Flex
                    alignItems={{ default: "alignItemsCenter" }}
                    gap={{ default: "gapSm" }}
                  >
                    <FlexItem>
                      <Title headingLevel="h2" size="lg">
                        Platform enforcement
                      </Title>
                    </FlexItem>
                    <FlexItem>
                      <ModeLabel enforced={mode === "enforce"} />
                    </FlexItem>
                    <FlexItem>
                      <ModeExplanationPopover platformEnforcement={mode} />
                    </FlexItem>
                  </Flex>
                </CardTitle>
              </CardHeader>
              <CardBody>
                <Stack hasGutter>
                  <StackItem>
                    <Content component="p">
                      {mode === "enforce"
                        ? "Every platform and gateway policy is enforced on every gateway."
                        : "Audit only is the minimum. A policy rejects operations only when its own setting is Enforced."}
                    </Content>
                  </StackItem>
                  {mode === "audit" ? (
                    <StackItem>
                      <DescriptionList isCompact isHorizontal>
                        <DescriptionListGroup>
                          <DescriptionListTerm>
                            Enforced by policy setting
                          </DescriptionListTerm>
                          <DescriptionListDescription>
                            1 of 2 policies
                          </DescriptionListDescription>
                        </DescriptionListGroup>
                      </DescriptionList>
                    </StackItem>
                  ) : null}
                </Stack>
              </CardBody>
              <CardFooter>
                <Button
                  onClick={() =>
                    setModal(
                      mode === "enforce"
                        ? "switch-to-audit"
                        : "switch-to-enforced",
                    )
                  }
                  variant="secondary"
                >
                  {mode === "enforce"
                    ? "Switch to Audit only..."
                    : "Switch to Enforced..."}
                </Button>
              </CardFooter>
            </Card>
          </GridItem>
          <GridItem lg={6}>
            <Card isFullHeight>
              <CardHeader>
                <CardTitle>
                  <Flex
                    alignItems={{ default: "alignItemsCenter" }}
                    gap={{ default: "gapSm" }}
                  >
                    <FlexItem>
                      <Title headingLevel="h2" size="lg">
                        Audit findings, last 24 hours
                      </Title>
                    </FlexItem>
                    {mode === "audit" && !isEmpty ? (
                      <FlexItem>
                        <HealthStatus
                          appearance="warning"
                          label="12 would be rejected"
                        />
                      </FlexItem>
                    ) : null}
                  </Flex>
                </CardTitle>
              </CardHeader>
              <CardBody>
                {mode === "enforce" || isEmpty ? (
                  <Content component="p">
                    No audit-only policies. Nothing to report.
                  </Content>
                ) : (
                  <DescriptionList
                    columnModifier={{ default: "1Col", md: "3Col" }}
                    displaySize="lg"
                  >
                    <DescriptionListGroup>
                      <DescriptionListTerm>Would reject</DescriptionListTerm>
                      <DescriptionListDescription>
                        <Button isInline variant="link">
                          12 operations
                        </Button>
                      </DescriptionListDescription>
                    </DescriptionListGroup>
                    <DescriptionListGroup>
                      <DescriptionListTerm>
                        Would add defaults
                      </DescriptionListTerm>
                      <DescriptionListDescription>
                        <Button isInline variant="link">
                          41 operations
                        </Button>
                      </DescriptionListDescription>
                    </DescriptionListGroup>
                    <DescriptionListGroup>
                      <DescriptionListTerm>
                        Gateways affected
                      </DescriptionListTerm>
                      <DescriptionListDescription>
                        3 of 14
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
                    Policies
                  </Title>
                </CardTitle>
              </CardHeader>
              <CardBody>
                {isEmpty ? (
                  <EmptyState
                    headingLevel="h3"
                    icon={ShieldAltIcon}
                    titleText="No platform policies"
                  >
                    <EmptyStateBody>
                      Create a policy to set sandbox rules on your gateways. New
                      policies start in audit only, so you can review findings
                      before anything is rejected.
                    </EmptyStateBody>
                    <EmptyStateFooter>
                      <EmptyStateActions>
                        <Button
                          component="a"
                          {...storyLinkProps(stories.createPlatformPolicy)}
                          variant="primary"
                        >
                          Create platform policy
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
                              aria-label="Filter policies by name"
                              placeholder="Filter by name"
                            />
                          </ToolbarItem>
                        </ToolbarContent>
                      </Toolbar>
                    </StackItem>
                    <StackItem>
                      <PoliciesTable mode={mode} onModal={setModal} />
                    </StackItem>
                  </Stack>
                )}
              </CardBody>
            </Card>
          </GridItem>
        </Grid>
      </PageSection>
      {modal ? (
        <PlatformPoliciesDialog
          modal={modal}
          onClose={() => setModal(undefined)}
        />
      ) : null}
    </>
  );
}
