import {
  Alert,
  AlertActionLink,
  Bullseye,
  Card,
  CardBody,
  CardTitle,
  Content,
  Flex,
  FlexItem,
  FormSelect,
  FormSelectOption,
  Grid,
  GridItem,
  PageSection,
  Spinner,
  Title,
} from "@patternfly/react-core";
import { UsersIcon } from "@patternfly/react-icons";
import { Fragment, useEffect, useState } from "react";
import { FormattedMessage, useIntl } from "react-intl";

import type {
  DashboardTimeRange,
  OperationalDashboardMetrics,
  OperationalMetric,
} from "../application/dashboard-types";
import { DashboardNavigation } from "../dashboard/dashboard-toolbar";
import { TrendSparklineChart } from "../dashboard/trend-sparkline-chart";
import { messages } from "../messages";
import { DashboardMetricUnavailable } from "./dashboard-widget";
import { DashboardPurpose } from "./dashboard-purpose";
import { useGetAdoptionMetricsData } from "./get-adoption-metrics-data";
import "./dashboard-widget.css";

function UnavailableMetricValue() {
  return (
    <span className="hypershell-adoption-dashboard__unavailable">
      <strong>
        <FormattedMessage {...messages.metricUnavailableTitle} />
      </strong>
      <small>
        <FormattedMessage {...messages.metricUnavailableBody} />
      </small>
    </span>
  );
}

function AdoptionSummaryCard({
  metric,
  timeRange,
}: Readonly<{
  metric: OperationalMetric | undefined;
  timeRange: DashboardTimeRange;
}>) {
  if (metric === undefined) {
    return <DashboardMetricUnavailable />;
  }

  const newRegistrations =
    timeRange === 7 ? metric.createdLast7Days : metric.createdLast30Days;

  return (
    <Content>
      <Title headingLevel="h3" size="lg">
        <FormattedMessage
          {...messages.adoptionRegisteredUsers}
          values={{ value: metric.value }}
        />
      </Title>
      <dl className="hypershell-adoption-dashboard__summary">
        <div>
          <dt>
            <FormattedMessage {...messages.adoptionNewRegistrations} />
          </dt>
          <dd>{newRegistrations ?? <UnavailableMetricValue />}</dd>
        </div>
        <div>
          <dt>
            <FormattedMessage {...messages.adoptionDailyActiveUsers} />
          </dt>
          <dd>{metric.dailyActiveUsers ?? <UnavailableMetricValue />}</dd>
        </div>
        <div>
          <dt>
            <FormattedMessage {...messages.adoptionWeeklyActiveUsers} />
          </dt>
          <dd>{metric.uniqueLoginsLast7Days ?? <UnavailableMetricValue />}</dd>
        </div>
        <div>
          <dt>
            <FormattedMessage {...messages.adoptionMonthlyActiveUsers} />
          </dt>
          <dd>{metric.uniqueLoginsLast30Days ?? <UnavailableMetricValue />}</dd>
        </div>
        <div>
          <dt>
            <FormattedMessage {...messages.adoptionInactiveUsers} />
          </dt>
          <dd>
            {metric.inactiveUsersLast30Days ?? <UnavailableMetricValue />}
          </dd>
        </div>
      </dl>
    </Content>
  );
}

function AdoptionConversionCard({
  metric,
}: Readonly<{
  metric: OperationalMetric | undefined;
}>) {
  if (metric === undefined) {
    return <DashboardMetricUnavailable />;
  }

  return (
    <Content>
      <Title headingLevel="h3" size="lg">
        {metric.gatewayConversionPercent ?? <UnavailableMetricValue />}
      </Title>
      <dl className="hypershell-adoption-dashboard__summary">
        <div>
          <dt>
            <FormattedMessage {...messages.adoptionFirstGatewayConversion} />
          </dt>
          <dd>{metric.usersWithGateway ?? <UnavailableMetricValue />}</dd>
        </div>
        <div>
          <dt>
            <FormattedMessage {...messages.adoptionFirstSandboxConversion} />
          </dt>
          <dd>
            <UnavailableMetricValue />
          </dd>
        </div>
        <div>
          <dt>
            <FormattedMessage {...messages.adoptionMedianTimeToFirstResource} />
          </dt>
          <dd>
            <UnavailableMetricValue />
          </dd>
        </div>
      </dl>
    </Content>
  );
}

function AdoptionTrendsCard({
  metric,
}: Readonly<{
  metric: OperationalMetric | undefined;
}>) {
  const intl = useIntl();

  if (metric === undefined) {
    return <DashboardMetricUnavailable />;
  }

  return (
    <Content>
      <p>
        <FormattedMessage {...messages.adoptionRegistrationTrend} />
      </p>
      <div className="hypershell-adoption-dashboard__trends">
        {metric.createdTrend ? (
          <TrendSparklineChart
            caption={intl.formatMessage(messages.adoptionTrendCaption)}
            title={intl.formatMessage(messages.adoptionNewRegistrations)}
            trend={metric.createdTrend}
          />
        ) : (
          <UnavailableMetricValue />
        )}
        {metric.trend ? (
          <TrendSparklineChart
            caption={intl.formatMessage(messages.adoptionTrendCaption)}
            title={intl.formatMessage(messages.adoptionDailyActiveUsers)}
            trend={metric.trend}
          />
        ) : (
          <UnavailableMetricValue />
        )}
      </div>
      <dl className="hypershell-adoption-dashboard__summary">
        <div>
          <dt>
            <FormattedMessage {...messages.adoptionRetention} />
          </dt>
          <dd>{metric.retention7DaysPercent ?? <UnavailableMetricValue />}</dd>
        </div>
        <div>
          <dt>
            <FormattedMessage {...messages.adoptionRetention30Days} />
          </dt>
          <dd>
            <UnavailableMetricValue />
          </dd>
        </div>
      </dl>
    </Content>
  );
}

export interface AdoptionDashboardPageProps {
  metrics?: OperationalDashboardMetrics;
  title?: string;
}

export function AdoptionDashboardPage({
  metrics,
  title,
}: Readonly<AdoptionDashboardPageProps>) {
  const intl = useIntl();
  const [timeRange, setTimeRange] = useState<DashboardTimeRange>(7);
  const [currentTime, setCurrentTime] = useState(0);
  const metricsQuery = useGetAdoptionMetricsData({
    enabled: metrics === undefined,
    timeRange,
  });
  const dashboardMetrics = metrics ?? metricsQuery.data;
  const metric = dashboardMetrics?.metrics.find(
    (candidate) => candidate.id === "adoption",
  );
  const updatedMinutesAgo = dashboardMetrics?.lastSuccessfulRefresh
    ? Math.max(
        0,
        Math.floor(
          (currentTime - dashboardMetrics.lastSuccessfulRefresh.getTime()) /
            60000,
        ),
      )
    : undefined;

  useEffect(() => {
    const intervalId = window.setInterval(() => {
      setCurrentTime(Date.now());
    }, 60_000);

    return () => {
      window.clearInterval(intervalId);
    };
  }, []);

  return (
    <Fragment>
      <PageSection
        data-page-region="header"
        hasBodyWrapper={false}
        variant="default"
      >
        <Content>
          <Title headingLevel="h1">
            {title ?? intl.formatMessage(messages.dashboardNavAdoption)}
          </Title>
          <p>
            <FormattedMessage {...messages.adoptionDescription} />
          </p>
        </Content>
      </PageSection>
      <PageSection
        data-page-region="body"
        hasBodyWrapper={false}
        isFilled
        variant="secondary"
      >
        <DashboardNavigation
          active="adoption"
          isRefreshing={metrics === undefined && metricsQuery.isFetching}
          onRefresh={() => {
            if (metrics === undefined) {
              void metricsQuery.refetch();
            }
          }}
          updatedMinutesAgo={updatedMinutesAgo}
        />
        <DashboardPurpose
          heading={<FormattedMessage {...messages.adoptionPurposeHeading} />}
        >
          <FormattedMessage {...messages.adoptionPurpose} />
        </DashboardPurpose>
        <Flex className="hypershell-adoption-dashboard__toolbar">
          <FlexItem>
            <FormSelect
              aria-label={intl.formatMessage(messages.timeRange)}
              value={String(timeRange)}
              onChange={(_event, value) => {
                setTimeRange(Number(value) as DashboardTimeRange);
              }}
            >
              <FormSelectOption
                value="7"
                label={intl.formatMessage(messages.timeRangeLast7Days)}
              />
              <FormSelectOption
                value="30"
                label={intl.formatMessage(messages.timeRangeLast30Days)}
              />
            </FormSelect>
          </FlexItem>
        </Flex>
        {metricsQuery.isPending && metrics === undefined ? (
          <Bullseye>
            <Spinner
              aria-label={intl.formatMessage(messages.adoptionLoading)}
            />
          </Bullseye>
        ) : null}
        {metrics === undefined && metricsQuery.isError && !metricsQuery.data ? (
          <Alert
            title={intl.formatMessage(messages.adoptionLoadErrorTitle)}
            variant="danger"
          >
            <FormattedMessage {...messages.adoptionLoadErrorBody} />
          </Alert>
        ) : null}
        {(dashboardMetrics?.failedSources?.length ?? 0) > 0 ? (
          <Alert
            actionLinks={
              <AlertActionLink
                isDisabled={metricsQuery.isFetching}
                onClick={() => {
                  void metricsQuery.refetch();
                }}
              >
                <FormattedMessage
                  {...messages.partialLoadWarningRefreshAction}
                />
              </AlertActionLink>
            }
            title={intl.formatMessage(messages.partialLoadWarningTitle)}
            variant="warning"
          >
            <FormattedMessage {...messages.partialLoadWarningBody} />
          </Alert>
        ) : null}
        <Grid hasGutter>
          <GridItem lg={6} xl={6} xl2={6}>
            <Card isFullHeight>
              <CardTitle>
                <UsersIcon />
                <FormattedMessage {...messages.adoptionWidgetSummary} />
              </CardTitle>
              <CardBody>
                <AdoptionSummaryCard metric={metric} timeRange={timeRange} />
              </CardBody>
            </Card>
          </GridItem>
          <GridItem lg={3} xl={3} xl2={3}>
            <Card isFullHeight>
              <CardTitle>
                <FormattedMessage {...messages.adoptionWidgetConversion} />
              </CardTitle>
              <CardBody>
                <AdoptionConversionCard metric={metric} />
              </CardBody>
            </Card>
          </GridItem>
          <GridItem lg={3} xl={3} xl2={3}>
            <Card isFullHeight>
              <CardTitle>
                <FormattedMessage {...messages.adoptionWidgetOrganizations} />
              </CardTitle>
              <CardBody>
                <DashboardMetricUnavailable />
              </CardBody>
            </Card>
          </GridItem>
          <GridItem span={12}>
            <Card isFullHeight>
              <CardTitle>
                <FormattedMessage {...messages.adoptionTrendsWidget} />
              </CardTitle>
              <CardBody>
                <AdoptionTrendsCard metric={metric} />
              </CardBody>
            </Card>
          </GridItem>
        </Grid>
      </PageSection>
    </Fragment>
  );
}
