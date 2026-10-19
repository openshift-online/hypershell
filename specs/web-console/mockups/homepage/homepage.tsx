import {
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
  Flex,
  FlexItem,
  Grid,
  GridItem,
  Icon,
  Label,
  List,
  ListItem,
  PageSection,
  Stack,
  StackItem,
  Title,
} from "@patternfly/react-core";
import {
  ArrowRightIcon,
  CheckCircleIcon,
  CubesIcon,
  ExclamationCircleIcon,
  ExclamationTriangleIcon,
  ExternalLinkAltIcon,
  HashtagIcon,
  ServerAltIcon,
} from "@patternfly/react-icons";

import { MockupTemplate } from "../shell/mockup-template";

export type HomepageState =
  | "loaded"
  | "partial"
  | "clusterError"
  | "clusterWarning"
  | "noAttention"
  | "platformRestricted";

type StatusTone = "success" | "warning" | "danger";

const statusIcon = {
  danger: ExclamationCircleIcon,
  success: CheckCircleIcon,
  warning: ExclamationTriangleIcon,
} satisfies Record<StatusTone, typeof CheckCircleIcon>;

function StatusHeading({
  children,
  tone,
}: {
  children: React.ReactNode;
  tone: StatusTone;
}) {
  const StatusIcon = statusIcon[tone];

  return (
    <Flex alignItems={{ default: "alignItemsCenter" }} gap={{ default: "gapSm" }}>
      <FlexItem>
        <Icon status={tone} title={tone}>
          <StatusIcon aria-hidden />
        </Icon>
      </FlexItem>
      <FlexItem>
        <CardTitle>{children}</CardTitle>
      </FlexItem>
    </Flex>
  );
}

function HomepageCard({
  children,
  footer,
  icon,
  showBody = true,
  title,
}: {
  children: React.ReactNode;
  footer?: React.ReactNode;
  icon: React.ReactNode;
  showBody?: boolean;
  title: React.ReactNode;
}) {
  return (
    <Card>
      <CardHeader>
        <Flex alignItems={{ default: "alignItemsCenter" }} gap={{ default: "gapSm" }}>
          <FlexItem>{icon}</FlexItem>
          <FlexItem>
            <CardTitle>{title}</CardTitle>
          </FlexItem>
        </Flex>
      </CardHeader>
      {showBody ? <CardBody>{children}</CardBody> : null}
      {footer ? <CardFooter>{footer}</CardFooter> : null}
    </Card>
  );
}

function WelcomeCard() {
  return (
    <Card>
      <CardBody>
        <Content>
          <Title headingLevel="h2" size="xl">
            Pick a gateway. Ship your sandboxes.
          </Title>
          <p className="pf-v6-u-pt-lg pf-v6-u-pb-lg">
            HyperShell runs OpenShell gateways across clusters and clouds for you. Choose where your workload belongs and get to a running gateway in just a few steps.
          </p>
        </Content>
        <Button component="a" href="/gateways/new" variant="primary">
          Provision gateway
        </Button>
      </CardBody>
    </Card>
  );
}

function YourGatewaysCard() {
  return (
    <HomepageCard icon={<CubesIcon />} title="Your gateways">
      <Flex alignItems={{ default: "alignItemsBaseline" }} gap={{ default: "gapMd" }}>
        <FlexItem>
          <Title headingLevel="h3" size="2xl">
            5
          </Title>
        </FlexItem>
        <FlexItem>gateways owned by you</FlexItem>
      </Flex>
      <Button
        component="a"
        href="/gateways"
        icon={<ArrowRightIcon />}
        variant="link"
      >
        View my gateways
      </Button>
    </HomepageCard>
  );
}

function NeedsAttentionCard({ state }: { state: HomepageState }) {
  if (state === "partial") {
    return (
      <HomepageCard
        icon={<ExclamationCircleIcon color="var(--pf-t--global--color--status--danger--default)" />}
        title="Needs attention"
      >
        <Label color="red">Gateway data unavailable</Label>
        <p>Gateway attention details could not be loaded.</p>
      </HomepageCard>
    );
  }

  if (state === "noAttention") {
    return (
      <HomepageCard
        icon={<CheckCircleIcon color="var(--pf-t--global--color--status--success--default)" />}
        showBody={false}
        title="All gateways are healthy"
      >
        {null}
      </HomepageCard>
    );
  }

  return (
    <HomepageCard
      icon={<ExclamationCircleIcon color="var(--pf-t--global--color--status--danger--default)" />}
      title="Needs attention"
    >
      <List>
        <ListItem>
          <Flex justifyContent={{ default: "justifyContentSpaceBetween" }}>
            <FlexItem>
              <Button component="a" href="/gateways/research-gateway" variant="link">
                research-gateway
              </Button>
              <Content component="small">Connection health is degraded</Content>
            </FlexItem>
            <FlexItem>
              <Flex alignItems={{ default: "alignItemsCenter" }} gap={{ default: "gapXs" }}>
                <Icon status="warning" title="Degraded">
                  <ExclamationTriangleIcon aria-hidden />
                </Icon>
                <span>Degraded</span>
              </Flex>
            </FlexItem>
          </Flex>
        </ListItem>
        <ListItem>
          <Flex justifyContent={{ default: "justifyContentSpaceBetween" }}>
            <FlexItem>
              <Button component="a" href="/gateways/team-gateway" variant="link">
                team-gateway
              </Button>
              <Content component="small">Provisioning failed</Content>
            </FlexItem>
            <FlexItem>
              <Flex alignItems={{ default: "alignItemsCenter" }} gap={{ default: "gapXs" }}>
                <Icon status="danger" title="Failed">
                  <ExclamationCircleIcon aria-hidden />
                </Icon>
                <span>Failed</span>
              </Flex>
            </FlexItem>
          </Flex>
        </ListItem>
      </List>
    </HomepageCard>
  );
}

function ClusterAvailabilityCard({ state }: { state: HomepageState }) {
  const clusterState =
    state === "clusterError"
      ? {
          name: "production-central",
          reason: "Cluster control plane is unreachable",
          tone: "danger" as const,
        }
      : state === "clusterWarning"
        ? {
            name: "research-east",
            reason: "Cluster connection health is degraded",
            tone: "warning" as const,
          }
        : null;

  return (
    <HomepageCard
      icon={
        state === "partial" ? (
          <ExclamationTriangleIcon color="var(--pf-t--global--color--status--warning--default)" />
        ) : clusterState ? (
          <ExclamationCircleIcon color="var(--pf-t--global--color--status--danger--default)" />
        ) : (
          <CheckCircleIcon color="var(--pf-t--global--color--status--success--default)" />
        )
      }
      title="Cluster availability"
    >
      {state === "partial" ? (
        <>
          <StatusHeading tone="warning">Cluster data unavailable</StatusHeading>
          <p>Cluster availability could not be loaded.</p>
        </>
      ) : clusterState ? (
        <Stack hasGutter>
          <StackItem>
            <List>
              <ListItem>
                <Flex justifyContent={{ default: "justifyContentSpaceBetween" }}>
                  <FlexItem>{clusterState.name}</FlexItem>
                <FlexItem>
                  <Flex alignItems={{ default: "alignItemsCenter" }} gap={{ default: "gapXs" }}>
                    <Icon
                      status={clusterState.tone}
                      title="Unavailable"
                    >
                      {clusterState.tone === "danger" ? (
                        <ExclamationCircleIcon aria-hidden />
                      ) : (
                        <ExclamationTriangleIcon aria-hidden />
                      )}
                    </Icon>
                    <span>Unavailable</span>
                  </Flex>
                </FlexItem>
                </Flex>
                <Content component="small">{clusterState.reason}</Content>
              </ListItem>
            </List>
          </StackItem>
          <StackItem>
            <Content component="small">
              These clusters may impact your gateways and sandboxes
            </Content>
          </StackItem>
        </Stack>
      ) : (
        <Stack hasGutter>
          <StackItem>
            <p>All 6 clusters are up and running</p>
          </StackItem>
          <StackItem>
            <Content component="small">Your gateways are running on healthy clusters.</Content>
          </StackItem>
        </Stack>
      )}
    </HomepageCard>
  );
}

function PlatformOverviewCard({ state }: { state: HomepageState }) {
  return (
    <HomepageCard
      footer={
        <Flex gap={{ default: "gapLg" }}>
          <FlexItem>
            <Button component="a" href="/dashboard" icon={<ExternalLinkAltIcon />} variant="link">
              Operational dashboard
            </Button>
          </FlexItem>
          <FlexItem>
            <Button component="a" href="/dashboard/reliability" icon={<ExternalLinkAltIcon />} variant="link">
              Reliability dashboard
            </Button>
          </FlexItem>
        </Flex>
      }
      icon={<ServerAltIcon />}
      title="Platform overview"
    >
      <Stack hasGutter>
        <StackItem>
          {state === "partial" ? (
            <Label color="grey">Platform inventory unavailable</Label>
          ) : (
            <DescriptionList columnModifier={{ default: "3Col", md: "3Col" }}>
              <DescriptionListGroup>
                <DescriptionListTerm>42</DescriptionListTerm>
                <DescriptionListDescription>gateways</DescriptionListDescription>
              </DescriptionListGroup>
              <DescriptionListGroup>
                <DescriptionListTerm>5</DescriptionListTerm>
                <DescriptionListDescription>
                  <Label
                    color="purple"
                    icon={<ExclamationCircleIcon />}
                    variant="outline"
                  >
                    Provisioning
                  </Label>
                </DescriptionListDescription>
              </DescriptionListGroup>
              <DescriptionListGroup>
                <DescriptionListTerm>2</DescriptionListTerm>
                <DescriptionListDescription>
                  <Label
                    color="red"
                    icon={<ExclamationCircleIcon />}
                    variant="outline"
                  >
                    Failed
                  </Label>
                </DescriptionListDescription>
              </DescriptionListGroup>
              <DescriptionListGroup>
                <DescriptionListTerm>128</DescriptionListTerm>
                <DescriptionListDescription>active sandboxes</DescriptionListDescription>
              </DescriptionListGroup>
            </DescriptionList>
          )}
        </StackItem>
        <StackItem>
          <Flex alignItems={{ default: "alignItemsCenter" }} gap={{ default: "gapXs" }}>
            <Icon
              status={state === "partial" ? "danger" : "danger"}
              title={state === "partial" ? "Unavailable" : "Clusters healthy"}
            >
              <ExclamationCircleIcon aria-hidden />
            </Icon>
            <span>
              {state === "partial" ? "Cluster health unavailable" : "5 of 6 clusters healthy"}
            </span>
          </Flex>
        </StackItem>
      </Stack>
    </HomepageCard>
  );
}

function SlackCard() {
  return (
    <Card>
      <CardBody>
        <Flex alignItems={{ default: "alignItemsCenter" }} gap={{ default: "gapLg" }} justifyContent={{ default: "justifyContentSpaceBetween" }}>
          <FlexItem>
            <HashtagIcon />
          </FlexItem>
          <FlexItem flex={{ default: "flex_1" }}>
            <Title headingLevel="h2" size="lg">
              Talk to the team in #forum-hypershell
            </Title>
            <Content>Onboarding, access to a gateway, questions, or something broken. We answer in the channel.</Content>
          </FlexItem>
          <FlexItem>
            <Button
              component="a"
              href="https://app.slack.com/client/E030G10V24F/C0BR5DM47Q9"
              target="_blank"
              variant="primary"
            >
              Open in Slack
            </Button>
          </FlexItem>
        </Flex>
      </CardBody>
    </Card>
  );
}

export function HomepagePageSurface({ state = "loaded" }: { state?: HomepageState }) {
  return <>
    <PageSection hasBodyWrapper={false}>
      <Title headingLevel="h1">Welcome to HyperShell</Title>
      <Content>A friendly starting point for your HyperShell work.</Content>
    </PageSection>
      <PageSection
        hasBodyWrapper={false}
        isFilled
        style={{
          backgroundColor:
            "var(--pf-t--global--background--color--secondary--default)",
        }}
        variant="secondary"
      >
        <Grid hasGutter>
          <GridItem span={12}>
            <WelcomeCard />
          </GridItem>
          <GridItem md={6} span={12}>
            <YourGatewaysCard />
          </GridItem>
          <GridItem md={6} span={12}>
            <NeedsAttentionCard state={state} />
          </GridItem>
          <GridItem md={6} span={12}>
            <ClusterAvailabilityCard state={state} />
          </GridItem>
          {state !== "platformRestricted" ? (
            <GridItem md={6} span={12}>
              <PlatformOverviewCard state={state} />
            </GridItem>
          ) : null}
          <GridItem span={12}>
            <SlackCard />
          </GridItem>
        </Grid>
      </PageSection>
  </>;
}

export function HomepageMockup({ state = "loaded" }: { state?: HomepageState }) {
  return (
    <MockupTemplate
      contentVariant="secondary"
      description="A friendly starting point for your HyperShell work."
      showBreadcrumbs={false}
      title="Welcome to HyperShell"
    >
      <HomepagePageSurface state={state} />
    </MockupTemplate>
  );
}
