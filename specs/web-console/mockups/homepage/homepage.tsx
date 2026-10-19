import {
  Alert,
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
  Progress,
  ProgressMeasureLocation,
  Split,
  SplitItem,
  Stack,
  StackItem,
  Title,
} from "@patternfly/react-core";
import {
  ArrowRightIcon,
  BookOpenIcon,
  ChartLineIcon,
  CheckCircleIcon,
  CodeIcon,
  CubesIcon,
  ExclamationCircleIcon,
  ExclamationTriangleIcon,
  HashtagIcon,
  RocketIcon,
  SearchIcon,
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
    <Flex
      alignItems={{ default: "alignItemsCenter" }}
      gap={{ default: "gapSm" }}
    >
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
        <Flex
          alignItems={{ default: "alignItemsCenter" }}
          gap={{ default: "gapSm" }}
        >
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
        <Split hasGutter>
          <SplitItem isFilled>
            <Content>
              <Title headingLevel="h2" size="xl">
                Pick a gateway. Ship your sandboxes.
              </Title>
              <p className="pf-v6-u-pt-lg pf-v6-u-pb-lg">
                HyperShell runs OpenShell gateways across clusters and clouds
                for you. Choose where your workload belongs and get to a running
                gateway in just a few steps.
              </p>
            </Content>
            <Flex gap={{ default: "gapMd" }}>
              <FlexItem>
                <Button
                  component="a"
                  href="/gateways/new"
                  icon={<RocketIcon />}
                  size="lg"
                  variant="primary"
                >
                  Provision my gateway
                </Button>
              </FlexItem>
              <FlexItem>
                <Button
                  component="a"
                  href="/gateways"
                  icon={<ArrowRightIcon />}
                  size="lg"
                  variant="secondary"
                >
                  View my gateways
                </Button>
              </FlexItem>
            </Flex>
          </SplitItem>
          <SplitItem>
            <Flex
              alignItems={{ default: "alignItemsCenter" }}
              direction={{ default: "column" }}
              gap={{ default: "gapLg" }}
            >
              <FlexItem>
                <Icon size="2xl">
                  <RocketIcon aria-hidden />
                </Icon>
              </FlexItem>
              <FlexItem>
                <Icon size="xl">
                  <CubesIcon aria-hidden />
                </Icon>
              </FlexItem>
              <FlexItem>
                <Icon size="lg">
                  <ChartLineIcon aria-hidden />
                </Icon>
              </FlexItem>
            </Flex>
          </SplitItem>
        </Split>
      </CardBody>
    </Card>
  );
}

function DashboardCard() {
  return (
    <HomepageCard icon={<ChartLineIcon />} title="Your dashboard">
      <Content>
        Track your gateways, sandbox activity, and reliability in one place.
      </Content>
      <Button
        component="a"
        href="/dashboard"
        icon={<ArrowRightIcon />}
        variant="link"
      >
        Go to your dashboard
      </Button>
    </HomepageCard>
  );
}

function GatewayHealthCard({ state }: { state: HomepageState }) {
  const status =
    state === "noAttention"
      ? {
          title: "All 5 gateways are healthy",
          tone: "success" as const,
        }
      : state === "partial"
        ? {
            title: "Gateway status is unavailable",
            tone: "warning" as const,
          }
        : {
            title: "2 gateways need attention",
            tone: "danger" as const,
          };

  return (
    <HomepageCard icon={<CubesIcon />} title="Your gateways">
      <Stack hasGutter>
        <StackItem>
          <Alert isInline title={status.title} variant={status.tone} />
        </StackItem>
        {state === "partial" ? (
          <StackItem>
            <Content>Gateway details could not be loaded.</Content>
          </StackItem>
        ) : (
          <>
            <StackItem>
              <Grid hasGutter>
                {state === "noAttention" ? (
                  <GridItem span={12}>
                    <Title headingLevel="h3" size="4xl">
                      5
                    </Title>
                    <Content>gateways</Content>
                  </GridItem>
                ) : (
                  <>
                    <GridItem md={4}>
                      <Title headingLevel="h3" size="4xl">
                        5
                      </Title>
                      <Content>gateways</Content>
                    </GridItem>
                    <GridItem md={4}>
                      <Title headingLevel="h3" size="2xl">
                        3
                      </Title>
                      <Content>healthy</Content>
                    </GridItem>
                    <GridItem md={4}>
                      <Title headingLevel="h3" size="2xl">
                        2
                      </Title>
                      <Content>need attention</Content>
                    </GridItem>
                  </>
                )}
              </Grid>
            </StackItem>
            {state === "noAttention" ? null : (
              <StackItem>
                <List>
                  <ListItem>
                    <Flex
                      justifyContent={{ default: "justifyContentSpaceBetween" }}
                    >
                      <FlexItem>
                        <Button
                          component="a"
                          href="/gateways/research-gateway"
                          variant="link"
                        >
                          research-gateway
                        </Button>
                        <Content component="small">
                          Connection health is degraded
                        </Content>
                      </FlexItem>
                      <FlexItem>
                        <Flex
                          alignItems={{ default: "alignItemsCenter" }}
                          gap={{ default: "gapXs" }}
                        >
                          <Icon status="warning" title="Degraded">
                            <ExclamationTriangleIcon aria-hidden />
                          </Icon>
                          <span>Degraded</span>
                        </Flex>
                      </FlexItem>
                    </Flex>
                  </ListItem>
                  <ListItem>
                    <Flex
                      justifyContent={{ default: "justifyContentSpaceBetween" }}
                    >
                      <FlexItem>
                        <Button
                          component="a"
                          href="/gateways/team-gateway"
                          variant="link"
                        >
                          team-gateway
                        </Button>
                        <Content component="small">Provisioning failed</Content>
                      </FlexItem>
                      <FlexItem>
                        <Flex
                          alignItems={{ default: "alignItemsCenter" }}
                          gap={{ default: "gapXs" }}
                        >
                          <Icon status="danger" title="Failed">
                            <ExclamationCircleIcon aria-hidden />
                          </Icon>
                          <span>Failed</span>
                        </Flex>
                      </FlexItem>
                    </Flex>
                  </ListItem>
                </List>
              </StackItem>
            )}
          </>
        )}
        <StackItem>
          <Button
            component="a"
            href="/gateways"
            icon={<ArrowRightIcon />}
            variant="link"
          >
            View my gateways
          </Button>
        </StackItem>
      </Stack>
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
      <Stack hasGutter>
        <StackItem>
          <Alert
            isInline
            title={
              state === "partial"
                ? "Cluster status is unavailable"
                : clusterState
                  ? `${clusterState.name} is unavailable`
                  : "All 6 clusters are healthy"
            }
            variant={
              state === "partial"
                ? "warning"
                : (clusterState?.tone ?? "success")
            }
          />
        </StackItem>
        {state === "partial" ? (
          <StackItem>
            <Content>Cluster availability could not be loaded.</Content>
          </StackItem>
        ) : (
          <StackItem>
            <Progress
              label={`${clusterState ? 5 : 6} of 6 clusters healthy`}
              max={6}
              measureLocation={ProgressMeasureLocation.outside}
              value={clusterState ? 5 : 6}
              variant={clusterState?.tone ?? "success"}
            />
          </StackItem>
        )}
        {clusterState ? (
          <>
            <StackItem>
              <List>
                <ListItem>
                  <Flex
                    justifyContent={{ default: "justifyContentSpaceBetween" }}
                  >
                    <FlexItem>{clusterState.name}</FlexItem>
                    <FlexItem>
                      <Flex
                        alignItems={{ default: "alignItemsCenter" }}
                        gap={{ default: "gapXs" }}
                      >
                        <Icon status={clusterState.tone} title="Unavailable">
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
          </>
        ) : (
          <StackItem>
            <Content component="small">
              Your gateways are running on healthy clusters.
            </Content>
          </StackItem>
        )}
      </Stack>
    </HomepageCard>
  );
}

function PlatformOverviewCard({ state }: { state: HomepageState }) {
  return (
    <HomepageCard
      footer={
        <Button
          component="a"
          href="/dashboard"
          icon={<ArrowRightIcon />}
          variant="link"
        >
          Dashboard
        </Button>
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
                <DescriptionListDescription>
                  gateways
                </DescriptionListDescription>
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
                <DescriptionListDescription>
                  active sandboxes
                </DescriptionListDescription>
              </DescriptionListGroup>
            </DescriptionList>
          )}
        </StackItem>
      </Stack>
    </HomepageCard>
  );
}

function LearnMoreCard() {
  const resources = [
    {
      description: "Get oriented with gateway concepts and workflows.",
      href: "/docs/openshell",
      icon: <BookOpenIcon />,
      title: "OpenShell docs",
    },
    {
      description: "Learn how HyperShell fits your platform.",
      href: "/docs/hypershell",
      icon: <RocketIcon />,
      title: "HyperShell overview",
    },
    {
      description: "Browse ready-to-run gateway examples.",
      href: "/examples",
      icon: <CodeIcon />,
      title: "Examples",
    },
    {
      description: "Find answers, patterns, and community discussion.",
      href: "/community",
      icon: <SearchIcon />,
      title: "Community",
    },
  ];

  return (
    <HomepageCard icon={<BookOpenIcon />} title="Learn and explore">
      <Grid hasGutter>
        {resources.map((resource) => (
          <GridItem key={resource.title} md={6}>
            <Flex
              alignItems={{ default: "alignItemsFlexStart" }}
              gap={{ default: "gapMd" }}
            >
              <FlexItem>{resource.icon}</FlexItem>
              <FlexItem flex={{ default: "flex_1" }}>
                <Button component="a" href={resource.href} variant="link">
                  {resource.title}
                </Button>
                <Content component="small">{resource.description}</Content>
              </FlexItem>
            </Flex>
          </GridItem>
        ))}
      </Grid>
    </HomepageCard>
  );
}

function SlackCard() {
  return (
    <Card>
      <CardBody>
        <Flex
          alignItems={{ default: "alignItemsCenter" }}
          gap={{ default: "gapLg" }}
          justifyContent={{ default: "justifyContentSpaceBetween" }}
        >
          <FlexItem>
            <HashtagIcon />
          </FlexItem>
          <FlexItem flex={{ default: "flex_1" }}>
            <Title headingLevel="h2" size="lg">
              Talk to the team in #forum-hypershell
            </Title>
            <Content>
              Onboarding, access to a gateway, questions, or something broken.
              We answer in the channel.
            </Content>
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

export function HomepagePageSurface({
  state = "loaded",
}: {
  state?: HomepageState;
}) {
  return (
    <>
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
          <GridItem xl={8}>
            <WelcomeCard />
          </GridItem>
          <GridItem xl={4}>
            {state === "platformRestricted" ? (
              <DashboardCard />
            ) : (
              <PlatformOverviewCard state={state} />
            )}
          </GridItem>
          <GridItem xl={8}>
            <GatewayHealthCard state={state} />
          </GridItem>
          <GridItem xl={4}>
            <ClusterAvailabilityCard state={state} />
          </GridItem>
          <GridItem xl={6}>
            <LearnMoreCard />
          </GridItem>
          <GridItem xl={6}>
            <SlackCard />
          </GridItem>
        </Grid>
      </PageSection>
    </>
  );
}

export function HomepageMockup({
  state = "loaded",
}: {
  state?: HomepageState;
}) {
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
