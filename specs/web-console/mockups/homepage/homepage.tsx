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
  Modal,
  ModalBody,
  ModalFooter,
  ModalHeader,
  ModalVariant,
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
  BoltIcon,
  ChartLineIcon,
  CheckCircleIcon,
  CodeIcon,
  CompassIcon,
  CubesIcon,
  ExclamationCircleIcon,
  ExclamationTriangleIcon,
  HashtagIcon,
  RocketIcon,
  ServerAltIcon,
  TerminalIcon,
} from "@patternfly/react-icons";
import { ChartDonut } from "@patternfly/react-charts/victory";

import { useId, useState } from "react";

import { MockupTemplate } from "../shell/mockup-template";

export type HomepageState =
  | "announcement"
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
  const quickCreateTitleId = useId();
  const [quickCreateOpen, setQuickCreateOpen] = useState(false);

  return (
    <>
      <Card>
        <CardBody className="pf-v6-u-p-3xl">
          <Split hasGutter>
            <SplitItem isFilled>
              <Content>
                <Title headingLevel="h2" size="xl">
                  Pick a gateway. Ship your sandboxes.
                </Title>
                <p className="pf-v6-u-pt-lg pf-v6-u-pb-lg">
                  HyperShell runs OpenShell gateways across clusters and clouds
                  for you. Choose where your workload belongs and get to a
                  running gateway in just a few steps.
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
                    Create a gateway
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
                    View gateways
                  </Button>
                </FlexItem>
                <FlexItem>
                  <Button
                    icon={<BoltIcon />}
                    onClick={() => setQuickCreateOpen(true)}
                    variant="link"
                  >
                    Quick create
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
              </Flex>
            </SplitItem>
          </Split>
        </CardBody>
      </Card>
      <Modal
        aria-labelledby={quickCreateTitleId}
        isOpen={quickCreateOpen}
        onClose={() => setQuickCreateOpen(false)}
        variant={ModalVariant.small}
      >
        <ModalHeader
          labelId={quickCreateTitleId}
          title="Quick create a gateway"
        />
        <ModalBody>
          <Content>
            Quick create makes a gateway on IBM Cloud public with a generated
            name. You can rename it after it starts.
          </Content>
        </ModalBody>
        <ModalFooter>
          <Button onClick={() => setQuickCreateOpen(false)} variant="primary">
            Create gateway
          </Button>
          <Button onClick={() => setQuickCreateOpen(false)} variant="link">
            Cancel
          </Button>
        </ModalFooter>
      </Modal>
    </>
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
  const healthyGateways =
    state === "noAttention" || state === "announcement" ? 5 : 3;
  const status =
    state === "noAttention" || state === "announcement"
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
    <HomepageCard icon={<CubesIcon />} title="Gateway health">
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
              <Progress
                label={`${healthyGateways} of 5 gateways healthy`}
                max={5}
                measureLocation={ProgressMeasureLocation.outside}
                value={healthyGateways}
                variant={status.tone}
              />
            </StackItem>
            {state === "noAttention" || state === "announcement" ? null : (
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

function ClusterAvailabilityContent({ state }: { state: HomepageState }) {
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
    <Stack hasGutter>
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
        </>
      ) : null}
    </Stack>
  );
}

type StatusDatum = {
  color: string;
  label: string;
  value: number;
};

const statusDonutTextStyle = {
  "--pf-v6-chart-donut--label--subtitle--Fill":
    "var(--pf-t--global--text--color--subtle)",
  "--pf-v6-chart-donut--label--title--Fill":
    "var(--pf-t--global--text--color--regular)",
  "--pf-v6-chart-global--label--Fill":
    "var(--pf-t--global--text--color--regular)",
} as React.CSSProperties;

function StatusDonut({
  ariaTitle,
  data,
  subTitle,
  title,
}: {
  ariaTitle: string;
  data: readonly StatusDatum[];
  subTitle: string;
  title: string;
}) {
  return (
    <div style={statusDonutTextStyle}>
      <ChartDonut
        ariaDesc={`${ariaTitle} by status`}
        ariaTitle={ariaTitle}
        colorScale={data.map((status) => status.color)}
        data={data.map((status) => ({ x: status.label, y: status.value }))}
        height={120}
        labels={({ datum }) => `${datum.x}: ${datum.y}`}
        legendData={data.map((status) => ({
          name: `${status.label}: ${status.value}`,
        }))}
        legendOrientation="vertical"
        legendPosition="right"
        padding={{ bottom: 8, left: 8, right: 100, top: 8 }}
        subTitle={subTitle}
        subTitlePosition="bottom"
        title={title}
        width={300}
      />
    </div>
  );
}

function PlatformOverviewCard({ state }: { state: HomepageState }) {
  const gatewayStatuses = [
    { label: "Healthy", value: 35, color: "#63993d" },
    { label: "Provisioning", value: 5, color: "#0066cc" },
    { label: "Failed", value: 2, color: "#c9190b" },
  ];
  const sandboxStatuses = [
    { label: "Active", value: 112, color: "#63993d" },
    { label: "Expiring", value: 9, color: "#f0ab00" },
    { label: "Idle", value: 5, color: "#0066cc" },
    { label: "Orphaned", value: 2, color: "#c9190b" },
  ];

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
            <>
              <StackItem>
                <Title headingLevel="h3" size="md">
                  Gateways
                </Title>
              </StackItem>
              <StatusDonut
                ariaTitle="Gateways"
                data={gatewayStatuses}
                subTitle="gateways"
                title="42"
              />
              <StackItem className="pf-v6-u-mt-md">
                <Title headingLevel="h3" size="md">
                  Active sandboxes
                </Title>
              </StackItem>
              <StatusDonut
                ariaTitle="Active sandboxes"
                data={sandboxStatuses}
                subTitle="sandboxes"
                title="128"
              />
            </>
          )}
        </StackItem>
        <StackItem>
          <Title headingLevel="h3" size="md">
            Cluster availability
          </Title>
        </StackItem>
        <StackItem>
          <ClusterAvailabilityContent state={state} />
        </StackItem>
      </Stack>
    </HomepageCard>
  );
}

function LearnMoreCard() {
  return (
    <HomepageCard icon={<CompassIcon />} title="Learn and explore">
      <List isPlain>
        <ListItem>
          <Flex
            alignItems={{ default: "alignItemsFlexStart" }}
            gap={{ default: "gapMd" }}
          >
            <FlexItem
              style={{
                paddingTop:
                  "var(--pf-t--global--spacer--control--vertical--default)",
              }}
            >
              <CodeIcon />
            </FlexItem>
            <FlexItem flex={{ default: "flex_1" }}>
              <Button
                component="a"
                href="https://github.com/openshift-online/hypershell/"
                target="_blank"
                variant="link"
              >
                HyperShell repo
              </Button>
              <Content component="small">
                Explore the source and project history.
              </Content>
            </FlexItem>
          </Flex>
        </ListItem>
        <ListItem>
          <Flex
            alignItems={{ default: "alignItemsFlexStart" }}
            gap={{ default: "gapMd" }}
          >
            <FlexItem
              style={{
                paddingTop:
                  "var(--pf-t--global--spacer--control--vertical--default)",
              }}
            >
              <BookOpenIcon />
            </FlexItem>
            <FlexItem flex={{ default: "flex_1" }}>
              <Button
                component="a"
                href="https://docs.nvidia.com/openshell/about/overview"
                target="_blank"
                variant="link"
              >
                OpenShell docs
              </Button>
              <Content component="small">
                Get oriented with OpenShell concepts.
              </Content>
            </FlexItem>
          </Flex>
        </ListItem>
        <ListItem>
          <Flex
            alignItems={{ default: "alignItemsFlexStart" }}
            gap={{ default: "gapMd" }}
          >
            <FlexItem
              style={{
                paddingTop:
                  "var(--pf-t--global--spacer--control--vertical--default)",
              }}
            >
              <TerminalIcon />
            </FlexItem>
            <FlexItem flex={{ default: "flex_1" }}>
              <Button
                component="a"
                href="https://docs.nvidia.com/openshell/about/installation"
                target="_blank"
                variant="link"
              >
                OpenShell CLI
              </Button>
              <Content component="small">
                Install and start using the CLI.
              </Content>
            </FlexItem>
          </Flex>
        </ListItem>
      </List>
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
        {state === "announcement" ? (
          <Alert
            className="pf-v6-u-mb-lg"
            title="Scheduled downtime"
            variant="info"
          >
            HyperShell will be unavailable on Oct 24 from 02:00 to 03:00 CT
            while we upgrade the platform.
          </Alert>
        ) : null}
        <Grid hasGutter>
          <GridItem xl={8}>
            <Stack hasGutter>
              <StackItem>
                <WelcomeCard />
              </StackItem>
              <StackItem>
                <GatewayHealthCard state={state} />
              </StackItem>
              <StackItem>
                <SlackCard />
              </StackItem>
            </Stack>
          </GridItem>
          <GridItem xl={4}>
            <Stack hasGutter>
              <StackItem>
                {state === "platformRestricted" ? (
                  <DashboardCard />
                ) : (
                  <PlatformOverviewCard state={state} />
                )}
              </StackItem>
              <StackItem>
                <LearnMoreCard />
              </StackItem>
            </Stack>
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
