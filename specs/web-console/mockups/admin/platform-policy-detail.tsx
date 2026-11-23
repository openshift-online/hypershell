import "@patternfly/react-core/dist/styles/base.css";

import {
  ActionList,
  ActionListItem,
  Button,
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
import { Table, Tbody, Td, Th, Thead, Tr } from "@patternfly/react-table";
import { ExternalLinkAltIcon } from "@patternfly/react-icons";
import { useState } from "react";

import { HealthStatus } from "../common/health-status";
import {
  baselinePolicy,
  findings,
  ModeLabel,
  ModeWithReason,
  OutcomeLabel,
  PolicyRulesTable,
} from "../common/policy-rules";

export type PlatformPolicyDetailTab = "overview" | "gateways" | "findings";

const rolloutPresentation = {
  applied: { appearance: "good" as const, label: "Applied" },
  pending: { appearance: "warning" as const, label: "Pending" },
  unsupported: { appearance: "danger" as const, label: "Cannot enforce" },
};

export function PlatformPolicyDetailPageSurface({
  activeTab: initialTab = "overview",
}: {
  activeTab?: PlatformPolicyDetailTab;
}) {
  const [activeTab, setActiveTab] = useState(initialTab);
  const policy = baselinePolicy;

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
                <ModeLabel enforced={false} />
              </FlexItem>
            </Flex>
            <Content component="p">
              Platform policy. Applies to {policy.appliesTo?.toLowerCase()}.
            </Content>
          </FlexItem>
          <FlexItem>
            <ActionList>
              <ActionListItem>
                <Button variant="primary">Start enforcing...</Button>
              </ActionListItem>
              <ActionListItem>
                <Button variant="secondary">Edit</Button>
              </ActionListItem>
              <ActionListItem>
                <Button isDanger variant="secondary">
                  Delete
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
        <Tabs
          activeKey={activeTab}
          aria-label="Policy overview, gateways, and audit findings"
          onSelect={(_event, key) =>
            setActiveTab(key as PlatformPolicyDetailTab)
          }
        >
          <Tab
            eventKey="overview"
            title={<TabTitleText>Overview</TabTitleText>}
          >
            <Grid hasGutter>
              <GridItem lg={7}>
                <Stack hasGutter>
                  <StackItem>
                    <Title headingLevel="h2" size="xl">
                      Rules
                    </Title>
                  </StackItem>
                  <StackItem>
                    <PolicyRulesTable name={policy.name} rules={policy.rules} />
                  </StackItem>
                  <StackItem>
                    <Title headingLevel="h2" size="xl">
                      Message for users
                    </Title>
                    <Content component="blockquote">
                      {policy.userMessage}
                    </Content>
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
                  </StackItem>
                </Stack>
              </GridItem>
              <GridItem lg={5}>
                <DescriptionList>
                  <DescriptionListGroup>
                    <DescriptionListTerm>Applies to</DescriptionListTerm>
                    <DescriptionListDescription>
                      {policy.appliesTo}
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                  <DescriptionListGroup>
                    <DescriptionListTerm>Enforcement</DescriptionListTerm>
                    <DescriptionListDescription>
                      Audit only (policy setting).
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                  <DescriptionListGroup>
                    <DescriptionListTerm>Applied</DescriptionListTerm>
                    <DescriptionListDescription>
                      <HealthStatus
                        appearance="danger"
                        label="Applied on 2 of 4 gateways"
                      />
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                  <DescriptionListGroup>
                    <DescriptionListTerm>Last updated</DescriptionListTerm>
                    <DescriptionListDescription>
                      {policy.updated} by {policy.updatedBy}
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                </DescriptionList>
              </GridItem>
            </Grid>
          </Tab>
          <Tab
            eventKey="gateways"
            title={<TabTitleText>Gateways</TabTitleText>}
          >
            <Table aria-label="Gateways this policy applies to">
              <Thead>
                <Tr>
                  <Th width={20}>Gateway</Th>
                  <Th width={15}>Applied</Th>
                  <Th width={10}>Revision</Th>
                  <Th width={20}>Enforcement</Th>
                  <Th modifier="wrap" width={10}>
                    Would reject, last 24 hours
                  </Th>
                  <Th width={25}>Detail</Th>
                </Tr>
              </Thead>
              <Tbody>
                {(policy.rollout ?? []).map((entry) => (
                  <Tr key={entry.gateway}>
                    <Td dataLabel="Gateway">
                      <Button
                        component="a"
                        href={`/gateways/${entry.gateway}`}
                        isInline
                        variant="link"
                      >
                        {entry.gateway}
                      </Button>
                    </Td>
                    <Td dataLabel="Applied">
                      <HealthStatus {...rolloutPresentation[entry.state]} />
                    </Td>
                    <Td dataLabel="Revision">{entry.revision}</Td>
                    <Td dataLabel="Enforcement">
                      <ModeWithReason
                        enforced={entry.enforced}
                        reason={entry.enforcementReason}
                      />
                    </Td>
                    <Td dataLabel="Would reject, last 24 hours">
                      {entry.enforced ? "-" : entry.wouldReject24h}
                    </Td>
                    <Td dataLabel="Detail">{entry.detail ?? "-"}</Td>
                  </Tr>
                ))}
              </Tbody>
            </Table>
          </Tab>
          <Tab
            eventKey="findings"
            title={<TabTitleText>Audit findings (12)</TabTitleText>}
          >
            <Table aria-label="Audit findings for this policy">
              <Thead>
                <Tr>
                  <Th width={20}>Time</Th>
                  <Th width={20}>Gateway</Th>
                  <Th width={15}>User</Th>
                  <Th width={15}>Outcome</Th>
                  <Th width={30}>Detail</Th>
                </Tr>
              </Thead>
              <Tbody>
                {findings.map((finding) => (
                  <Tr key={`${finding.time}-${finding.user}`}>
                    <Td dataLabel="Time">{finding.time}</Td>
                    <Td dataLabel="Gateway">{finding.gateway}</Td>
                    <Td dataLabel="User">{finding.user}</Td>
                    <Td dataLabel="Outcome">
                      <OutcomeLabel outcome={finding.outcome} />
                    </Td>
                    <Td dataLabel="Detail">{finding.detail}</Td>
                  </Tr>
                ))}
              </Tbody>
            </Table>
          </Tab>
        </Tabs>
      </PageSection>
    </>
  );
}
