import {
  Alert,
  Button,
  Card,
  CardBody,
  CardFooter,
  CardHeader,
  CardTitle,
  Content,
  Flex,
  FlexItem,
  Grid,
  GridItem,
  Icon,
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
  Spinner,
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
import { FormattedMessage, useIntl } from "react-intl";
import { Link, useNavigate } from "react-router";
import { useId, useState } from "react";

import { externalLinkUrls } from "@openshift-online/hypershell-gateway-management-ui";
import type {
  HomepagePlatformOverview,
  HomepageSummary,
} from "../../adapters/api/homepage";
import { messages } from "../../i18n/messages";
import { gatewayDetailPath, routePaths } from "../../../shared/routes";
import {
  homepageAnnouncement,
  type HomepageAnnouncement,
} from "./announcement";
import {
  useHomepagePlatformOverview,
  useHomepageSummary,
  useQuickCreateGateway,
} from "./use-homepage";
import { useSession } from "../shell/use-session";
import { hasDashboardAdminRole } from "../../lib/session-roles";

type StatusTone = "success" | "warning" | "danger";

interface StatusDatum {
  color: string;
  label: string;
  value: number;
}

const statusDonutTextStyle = {
  "--pf-v6-chart-donut--label--subtitle--Fill":
    "var(--pf-t--global--text--color--subtle)",
  "--pf-v6-chart-donut--label--title--Fill":
    "var(--pf-t--global--text--color--regular)",
  "--pf-v6-chart-global--label--Fill":
    "var(--pf-t--global--text--color--regular)",
} as React.CSSProperties;

const chartColors: Record<string, string> = {
  Active: "var(--pf-t--global--color--status--success--default)",
  Degraded: "#f0ab00",
  Expiring: "#f0ab00",
  Failed: "#c9190b",
  Healthy: "var(--pf-t--global--color--status--success--default)",
  Idle: "#0066cc",
  Orphaned: "#c9190b",
  Pending: "#0066cc",
  Provisioning: "#0066cc",
  Running: "var(--pf-t--global--color--status--success--default)",
  active: "var(--pf-t--global--color--status--success--default)",
  expiring: "#f0ab00",
  idle: "#0066cc",
  orphaned: "#c9190b",
};

function statusTone(status: string): StatusTone {
  const normalized = status.trim().toLowerCase();
  if (["failed", "error", "orphaned"].includes(normalized)) {
    return "danger";
  }
  if (["degraded", "expiring", "unavailable", "warning"].includes(normalized)) {
    return "warning";
  }
  return "success";
}

function StatusIcon({ status }: { status: string }) {
  const tone = statusTone(status);
  const IconComponent =
    tone === "danger" ? ExclamationCircleIcon : ExclamationTriangleIcon;
  return (
    <Icon status={tone === "success" ? "success" : tone} title={status}>
      <IconComponent aria-hidden />
    </Icon>
  );
}

function HomepageCard({
  children,
  footer,
  icon,
  title,
}: Readonly<{
  children: React.ReactNode;
  footer?: React.ReactNode;
  icon: React.ReactNode;
  title: React.ReactNode;
}>) {
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
      <CardBody>{children}</CardBody>
      {footer ? <CardFooter>{footer}</CardFooter> : null}
    </Card>
  );
}

function LoadingContent() {
  const intl = useIntl();
  return (
    <Flex
      alignItems={{ default: "alignItemsCenter" }}
      gap={{ default: "gapSm" }}
    >
      <FlexItem>
        <Spinner
          size="lg"
          aria-label={intl.formatMessage(messages.homepageLoading)}
        />
      </FlexItem>
      <FlexItem>
        <Content>
          <FormattedMessage {...messages.homepageLoading} />
        </Content>
      </FlexItem>
    </Flex>
  );
}

function UnavailableContent({ title }: { title: string }) {
  return (
    <Alert isInline title={title} variant="warning">
      <FormattedMessage {...messages.homepageDataUnavailableBody} />
    </Alert>
  );
}

function WelcomeCard({
  quickCreatePlacement,
  onQuickCreate,
  quickCreateError,
  quickCreateIsPending,
  quickCreateOpen,
  setQuickCreateOpen,
}: Readonly<{
  quickCreatePlacement?: HomepageSummary["quickCreate"];
  onQuickCreate: () => void;
  quickCreateError: boolean;
  quickCreateIsPending: boolean;
  quickCreateOpen: boolean;
  setQuickCreateOpen: (open: boolean) => void;
}>) {
  const quickCreateTitleId = useId();
  const placement = quickCreatePlacement?.placement;
  const isLocalKind = placement !== undefined && "mode" in placement;
  return (
    <>
      <Card>
        <CardBody className="pf-v6-u-p-3xl">
          <Split hasGutter>
            <SplitItem isFilled>
              <Content>
                <Title headingLevel="h2" size="xl">
                  <FormattedMessage {...messages.homepageWelcomeTitle} />
                </Title>
                <p className="pf-v6-u-pt-lg pf-v6-u-pb-lg">
                  <FormattedMessage {...messages.homepageWelcomeBody} />
                </p>
              </Content>
              <Flex gap={{ default: "gapMd" }}>
                <FlexItem>
                  <Button
                    component={Link}
                    {...{ to: routePaths.gatewayNew }}
                    icon={<RocketIcon />}
                    size="lg"
                    variant="primary"
                  >
                    <FormattedMessage {...messages.homepageCreateGateway} />
                  </Button>
                </FlexItem>
                <FlexItem>
                  <Button
                    component={Link}
                    {...{ to: routePaths.gatewayList }}
                    icon={<ArrowRightIcon />}
                    size="lg"
                    variant="secondary"
                  >
                    <FormattedMessage {...messages.homepageViewGateways} />
                  </Button>
                </FlexItem>
                {quickCreatePlacement !== undefined ? (
                  <FlexItem>
                    <Button
                      icon={<BoltIcon />}
                      onClick={() => {
                        setQuickCreateOpen(true);
                      }}
                      variant="link"
                    >
                      <FormattedMessage {...messages.homepageQuickCreate} />
                    </Button>
                  </FlexItem>
                ) : null}
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
        onClose={() => {
          setQuickCreateOpen(false);
        }}
        variant={ModalVariant.small}
      >
        <ModalHeader
          labelId={quickCreateTitleId}
          title={<FormattedMessage {...messages.homepageQuickCreateTitle} />}
        />
        <ModalBody>
          <Content>
            <FormattedMessage
              {...(isLocalKind
                ? messages.homepageQuickCreateLocalKindBody
                : messages.homepageQuickCreateBody)}
            />
          </Content>
          {quickCreateError ? (
            <Alert
              className="pf-v6-u-mt-md"
              isInline
              title={
                <FormattedMessage {...messages.homepageQuickCreateError} />
              }
              variant="danger"
            />
          ) : null}
        </ModalBody>
        <ModalFooter>
          <Button
            isDisabled={quickCreateIsPending}
            isLoading={quickCreateIsPending}
            onClick={onQuickCreate}
            variant="primary"
          >
            <FormattedMessage {...messages.homepageQuickCreateSubmit} />
          </Button>
          <Button
            isDisabled={quickCreateIsPending}
            onClick={() => {
              setQuickCreateOpen(false);
            }}
            variant="link"
          >
            <FormattedMessage {...messages.homepageCancel} />
          </Button>
        </ModalFooter>
      </Modal>
    </>
  );
}

function GatewayHealthCard({
  isLoading,
  summary,
}: Readonly<{
  isLoading: boolean;
  summary?: HomepageSummary;
}>) {
  const intl = useIntl();
  const health = summary?.gatewayHealth;
  const unavailable = health === undefined || "available" in health;
  const availableHealth = unavailable ? undefined : health;
  const attentionCount = availableHealth?.attentionCount ?? 0;
  const total = availableHealth?.total ?? 0;
  const healthy = availableHealth?.healthy ?? 0;
  const tone: StatusTone = attentionCount > 0 ? "danger" : "success";

  return (
    <HomepageCard
      icon={<CubesIcon />}
      title={<FormattedMessage {...messages.homepageGatewayHealthTitle} />}
    >
      <Stack hasGutter>
        <StackItem>
          {isLoading ? (
            <LoadingContent />
          ) : unavailable ? (
            <UnavailableContent
              title={intl.formatMessage(
                messages.homepageGatewayHealthUnavailable,
              )}
            />
          ) : (
            <Alert
              isInline
              title={
                attentionCount === 0 ? (
                  <FormattedMessage
                    {...messages.homepageAllGatewaysHealthy}
                    values={{ total }}
                  />
                ) : (
                  <FormattedMessage
                    {...messages.homepageGatewaysNeedAttention}
                    values={{ count: attentionCount }}
                  />
                )
              }
              variant={tone}
            />
          )}
        </StackItem>
        {!unavailable ? (
          <>
            <StackItem>
              <Progress
                aria-label={intl.formatMessage(
                  messages.homepageGatewayProgress,
                  {
                    healthy,
                    total,
                  },
                )}
                label={intl.formatMessage(messages.homepageGatewayProgress, {
                  healthy,
                  total,
                })}
                max={total}
                measureLocation={ProgressMeasureLocation.outside}
                value={healthy}
                variant={tone}
              />
            </StackItem>
            {attentionCount > 0 ? (
              <StackItem>
                <List>
                  {availableHealth?.attention.map((gateway) => (
                    <ListItem key={gateway.gatewayId}>
                      <Flex
                        justifyContent={{
                          default: "justifyContentSpaceBetween",
                        }}
                      >
                        <FlexItem>
                          <Button
                            component={Link}
                            {...{
                              to: gatewayDetailPath(gateway.gatewayId),
                            }}
                            variant="link"
                          >
                            {gateway.name}
                          </Button>
                          <Content component="small">{gateway.reason}</Content>
                        </FlexItem>
                        <FlexItem>
                          <Flex
                            alignItems={{ default: "alignItemsCenter" }}
                            gap={{ default: "gapXs" }}
                          >
                            <StatusIcon status={gateway.status} />
                            <span>{gateway.status}</span>
                          </Flex>
                        </FlexItem>
                      </Flex>
                    </ListItem>
                  ))}
                </List>
              </StackItem>
            ) : null}
          </>
        ) : null}
        <StackItem>
          <Button
            component={Link}
            {...{ to: routePaths.gatewayList }}
            icon={<ArrowRightIcon />}
            variant="link"
          >
            <FormattedMessage {...messages.homepageViewMyGateways} />
          </Button>
        </StackItem>
      </Stack>
    </HomepageCard>
  );
}

function DashboardCard() {
  return (
    <HomepageCard
      icon={<ChartLineIcon />}
      title={<FormattedMessage {...messages.homepageYourDashboardTitle} />}
    >
      <Content>
        <FormattedMessage {...messages.homepageYourDashboardBody} />
      </Content>
      <Button
        component={Link}
        {...{ to: routePaths.dashboard }}
        icon={<ArrowRightIcon />}
        variant="link"
      >
        <FormattedMessage {...messages.homepageGoToDashboard} />
      </Button>
    </HomepageCard>
  );
}

function StatusDonut({
  ariaTitle,
  counts,
  subTitle,
  title,
}: Readonly<{
  ariaTitle: string;
  counts: Record<string, number>;
  subTitle: string;
  title: string;
}>) {
  const data: StatusDatum[] = Object.entries(counts).map(([label, value]) => ({
    color: chartColors[label] ?? "#8a8d90",
    label,
    value,
  }));
  const visibleData = data.filter((status) => status.value > 0);
  return (
    <div
      style={{
        ...statusDonutTextStyle,
        height: 120,
        width: 300,
      }}
    >
      <ChartDonut
        ariaDesc={`${ariaTitle} by status`}
        ariaTitle={ariaTitle}
        colorScale={visibleData.map((status) => status.color)}
        data={visibleData.map((status) => ({
          x: status.label,
          y: status.value,
        }))}
        height={120}
        labels={({ datum }: { datum: { x?: string | number; y?: number } }) =>
          `${String(datum.x ?? "")}:${String(datum.y ?? 0)}`
        }
        legendData={visibleData.map((status) => ({
          name: `${status.label}:${String(status.value)}`,
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

function PlatformOverviewCard({
  isLoading,
  overview,
}: Readonly<{
  isLoading: boolean;
  overview?: HomepagePlatformOverview;
}>) {
  const intl = useIntl();
  const clusters = overview?.clusters;
  const clusterUnavailable = clusters === undefined || "available" in clusters;
  const availableClusters = clusterUnavailable ? undefined : clusters;
  const gatewayStatuses = overview?.gatewayStatuses;
  const sandboxStatuses = overview?.sandboxStatuses;

  return (
    <HomepageCard
      footer={
        <Button
          component={Link}
          {...{ to: routePaths.dashboard }}
          icon={<ArrowRightIcon />}
          variant="link"
        >
          <FormattedMessage {...messages.homepageDashboard} />
        </Button>
      }
      icon={<ServerAltIcon />}
      title={<FormattedMessage {...messages.homepagePlatformOverviewTitle} />}
    >
      <Stack hasGutter>
        {isLoading ? <LoadingContent /> : null}
        {!isLoading && gatewayStatuses?.available ? (
          <>
            <StackItem>
              <Title headingLevel="h3" size="md">
                <FormattedMessage {...messages.homepageGatewaysChartTitle} />
              </Title>
            </StackItem>
            <StackItem>
              <StatusDonut
                ariaTitle={intl.formatMessage(
                  messages.homepageGatewaysChartTitle,
                )}
                counts={gatewayStatuses.counts}
                subTitle={intl.formatMessage(messages.homepageGatewaysSubTitle)}
                title={String(gatewayStatuses.total)}
              />
            </StackItem>
          </>
        ) : null}
        {!isLoading && !gatewayStatuses?.available ? (
          <StackItem>
            <UnavailableContent
              title={intl.formatMessage(
                messages.homepagePlatformOverviewUnavailable,
              )}
            />
          </StackItem>
        ) : null}
        {!isLoading && sandboxStatuses?.available ? (
          <>
            <StackItem>
              <Title headingLevel="h3" size="md">
                <FormattedMessage {...messages.homepageSandboxesChartTitle} />
              </Title>
            </StackItem>
            <StackItem>
              <StatusDonut
                ariaTitle={intl.formatMessage(
                  messages.homepageSandboxesChartTitle,
                )}
                counts={sandboxStatuses.counts}
                subTitle={intl.formatMessage(
                  messages.homepageSandboxesSubTitle,
                )}
                title={String(sandboxStatuses.total)}
              />
            </StackItem>
          </>
        ) : null}
        {!isLoading && !sandboxStatuses?.available ? (
          <StackItem>
            <UnavailableContent
              title={intl.formatMessage(
                messages.homepagePlatformOverviewUnavailable,
              )}
            />
          </StackItem>
        ) : null}
        <StackItem>
          <Title headingLevel="h3" size="md">
            <FormattedMessage {...messages.homepageClusterAvailabilityTitle} />
          </Title>
        </StackItem>
        {!clusterUnavailable ? (
          <>
            <StackItem>
              <Progress
                aria-label={intl.formatMessage(
                  messages.homepageClusterProgress,
                  {
                    healthy: availableClusters?.healthy ?? 0,
                    total: availableClusters?.total ?? 0,
                  },
                )}
                label={intl.formatMessage(messages.homepageClusterProgress, {
                  healthy: availableClusters?.healthy ?? 0,
                  total: availableClusters?.total ?? 0,
                })}
                max={clusters.total}
                measureLocation={ProgressMeasureLocation.outside}
                value={clusters.healthy}
                variant={
                  availableClusters?.healthy === availableClusters?.total
                    ? "success"
                    : "warning"
                }
              />
            </StackItem>
            {availableClusters?.unhealthyClusters.map((cluster) => (
              <StackItem key={`${cluster.name}-${cluster.status}`}>
                <Flex
                  justifyContent={{ default: "justifyContentSpaceBetween" }}
                >
                  <FlexItem>{cluster.name}</FlexItem>
                  <FlexItem>
                    <Flex
                      alignItems={{ default: "alignItemsCenter" }}
                      gap={{ default: "gapXs" }}
                    >
                      <StatusIcon status={cluster.status} />
                      <span>
                        <FormattedMessage
                          {...messages.homepageClusterUnavailableStatus}
                        />
                      </span>
                    </Flex>
                  </FlexItem>
                </Flex>
                <Content component="small">{cluster.reason}</Content>
              </StackItem>
            ))}
          </>
        ) : (
          <StackItem>
            <UnavailableContent
              title={intl.formatMessage(
                messages.homepagePlatformOverviewUnavailable,
              )}
            />
          </StackItem>
        )}
      </Stack>
    </HomepageCard>
  );
}

function LearnCard() {
  return (
    <HomepageCard
      icon={<CompassIcon />}
      title={<FormattedMessage {...messages.homepageLearnTitle} />}
    >
      <List isPlain>
        <ListItem>
          <Flex
            alignItems={{ default: "alignItemsFlexStart" }}
            gap={{ default: "gapMd" }}
          >
            <FlexItem>
              <CodeIcon />
            </FlexItem>
            <FlexItem flex={{ default: "flex_1" }}>
              <Button
                component="a"
                href={externalLinkUrls.hyperShellRepository}
                target="_blank"
                rel="noreferrer"
                variant="link"
              >
                <FormattedMessage {...messages.homepageHyperShellRepository} />
              </Button>
              <Content component="small">
                <FormattedMessage
                  {...messages.homepageHyperShellRepositoryDescription}
                />
              </Content>
            </FlexItem>
          </Flex>
        </ListItem>
        <ListItem>
          <Flex
            alignItems={{ default: "alignItemsFlexStart" }}
            gap={{ default: "gapMd" }}
          >
            <FlexItem>
              <BookOpenIcon />
            </FlexItem>
            <FlexItem flex={{ default: "flex_1" }}>
              <Button
                component="a"
                href={externalLinkUrls.openShellDocs}
                target="_blank"
                rel="noreferrer"
                variant="link"
              >
                <FormattedMessage {...messages.homepageOpenShellDocs} />
              </Button>
              <Content component="small">
                <FormattedMessage
                  {...messages.homepageOpenShellDocsDescription}
                />
              </Content>
            </FlexItem>
          </Flex>
        </ListItem>
        <ListItem>
          <Flex
            alignItems={{ default: "alignItemsFlexStart" }}
            gap={{ default: "gapMd" }}
          >
            <FlexItem>
              <TerminalIcon />
            </FlexItem>
            <FlexItem flex={{ default: "flex_1" }}>
              <Button
                component="a"
                href={externalLinkUrls.openShellCliInstallation}
                target="_blank"
                rel="noreferrer"
                variant="link"
              >
                <FormattedMessage {...messages.homepageOpenShellCli} />
              </Button>
              <Content component="small">
                <FormattedMessage
                  {...messages.homepageOpenShellCliDescription}
                />
              </Content>
            </FlexItem>
          </Flex>
        </ListItem>
      </List>
    </HomepageCard>
  );
}

function SupportCard() {
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
              <FormattedMessage {...messages.homepageSupportTitle} />
            </Title>
            <Content>
              <FormattedMessage {...messages.homepageSupportDescription} />
            </Content>
          </FlexItem>
          <FlexItem>
            <Button
              component="a"
              href={externalLinkUrls.supportSlackChannel}
              target="_blank"
              rel="noreferrer"
              variant="primary"
            >
              <FormattedMessage {...messages.homepageSupportOpenAction} />
            </Button>
          </FlexItem>
        </Flex>
      </CardBody>
    </Card>
  );
}

export function HomepagePageSurface({
  announcement = homepageAnnouncement,
  isAdmin,
  platformOverview,
  platformOverviewIsLoading,
  quickCreatePlacement,
  quickCreate,
  quickCreateError,
  quickCreateIsPending,
  quickCreateOpen,
  sessionIsLoading,
  setQuickCreateOpen,
  summary,
  summaryIsLoading,
}: Readonly<{
  announcement?: HomepageAnnouncement | null;
  isAdmin: boolean;
  platformOverview?: HomepagePlatformOverview;
  platformOverviewIsLoading: boolean;
  quickCreatePlacement?: HomepageSummary["quickCreate"];
  quickCreate: () => void;
  quickCreateError: boolean;
  quickCreateIsPending: boolean;
  quickCreateOpen: boolean;
  sessionIsLoading: boolean;
  setQuickCreateOpen: (open: boolean) => void;
  summary?: HomepageSummary;
  summaryIsLoading: boolean;
}>) {
  return (
    <PageSection
      hasBodyWrapper={false}
      isFilled
      style={{
        backgroundColor:
          "var(--pf-t--global--background--color--secondary--default)",
      }}
      variant="secondary"
    >
      {announcement ? (
        <Alert
          className="pf-v6-u-mb-lg"
          isInline
          title={<FormattedMessage {...announcement.title} />}
          variant="info"
        >
          <FormattedMessage {...announcement.body} />
        </Alert>
      ) : null}
      <Grid hasGutter>
        <GridItem xl={8}>
          <Stack hasGutter>
            <StackItem>
              <WelcomeCard
                onQuickCreate={quickCreate}
                quickCreatePlacement={quickCreatePlacement}
                quickCreateError={quickCreateError}
                quickCreateIsPending={quickCreateIsPending}
                quickCreateOpen={quickCreateOpen}
                setQuickCreateOpen={setQuickCreateOpen}
              />
            </StackItem>
            <StackItem>
              <GatewayHealthCard
                isLoading={summaryIsLoading}
                summary={summary}
              />
            </StackItem>
            <StackItem>
              <SupportCard />
            </StackItem>
          </Stack>
        </GridItem>
        <GridItem xl={4}>
          <Stack hasGutter>
            <StackItem>
              {sessionIsLoading ? (
                <HomepageCard
                  icon={<ChartLineIcon />}
                  title={
                    <FormattedMessage {...messages.homepageRoleLoadingTitle} />
                  }
                >
                  <LoadingContent />
                </HomepageCard>
              ) : isAdmin ? (
                <PlatformOverviewCard
                  isLoading={platformOverviewIsLoading}
                  overview={platformOverview}
                />
              ) : (
                <DashboardCard />
              )}
            </StackItem>
            <StackItem>
              <LearnCard />
            </StackItem>
          </Stack>
        </GridItem>
      </Grid>
    </PageSection>
  );
}

export function HomepagePage() {
  const navigate = useNavigate();
  const sessionQuery = useSession();
  const summaryQuery = useHomepageSummary();
  const quickCreatePlacement = summaryQuery.data?.quickCreate;
  const platformOverviewQuery = useHomepagePlatformOverview();
  const [quickCreateOpen, setQuickCreateOpen] = useState(false);
  const quickCreate = useQuickCreateGateway(
    (gatewayId) => {
      setQuickCreateOpen(false);
      void navigate(gatewayDetailPath(gatewayId));
    },
    quickCreateOpen,
    quickCreatePlacement?.placement,
  );

  return (
    <HomepagePageSurface
      announcement={homepageAnnouncement}
      isAdmin={hasDashboardAdminRole(sessionQuery.data?.roles ?? [])}
      platformOverview={platformOverviewQuery.data}
      platformOverviewIsLoading={platformOverviewQuery.isPending}
      quickCreatePlacement={quickCreatePlacement}
      quickCreate={() => {
        quickCreate.mutate();
      }}
      quickCreateError={quickCreate.isError}
      quickCreateIsPending={quickCreate.isPending}
      quickCreateOpen={quickCreateOpen}
      sessionIsLoading={sessionQuery.isPending}
      setQuickCreateOpen={setQuickCreateOpen}
      summary={summaryQuery.data}
      summaryIsLoading={summaryQuery.isPending}
    />
  );
}
