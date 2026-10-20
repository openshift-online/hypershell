import { Button, Content, PageSection, Title } from "@patternfly/react-core";
import type { ExtendedTemplateConfig } from "@patternfly/widgetized-dashboard";
import {
  ChartLineIcon,
  CheckCircleIcon,
  ExclamationCircleIcon,
  OutlinedClockIcon,
  PlusCircleIcon,
  SyncAltIcon,
  TachometerAltIcon,
} from "@patternfly/react-icons";
import { MockupShell } from "../shell/mockup-template";
import {
  DashboardSummaryList,
  DashboardSparkline,
  DashboardWidget,
  MockupWidgetizedLayout,
} from "../common/dashboard-widget";
import {
  DashboardFilters,
  DashboardNavigation,
  DashboardPurpose,
} from "../common/dashboard-controls";

const reliabilityLayout: ExtendedTemplateConfig = {
  xl: [
    {
      h: 4,
      i: "reliability-summary#1",
      title: "Reliability summary",
      w: 3,
      x: 0,
      y: 0,
      widgetType: "reliability-summary",
    },
    {
      h: 5,
      i: "api-request-rate#1",
      title: "API request rate",
      w: 1,
      x: 0,
      y: 4,
      widgetType: "api-request-rate",
    },
    {
      h: 5,
      i: "api-error-rate#1",
      title: "API error rate",
      w: 1,
      x: 1,
      y: 4,
      widgetType: "api-error-rate",
    },
    {
      h: 5,
      i: "api-latency#1",
      title: "API latency",
      w: 1,
      x: 2,
      y: 4,
      widgetType: "api-latency",
    },
    {
      h: 5,
      i: "reconciliation-failures#1",
      title: "Reconciliation failures",
      w: 1,
      x: 0,
      y: 9,
      widgetType: "reconciliation-failures",
    },
    {
      h: 5,
      i: "reconciliation-retries#1",
      title: "Reconciliation retries",
      w: 1,
      x: 1,
      y: 9,
      widgetType: "reconciliation-retries",
    },
    {
      h: 5,
      i: "reconciliation-lag#1",
      title: "Reconciliation lag",
      w: 1,
      x: 2,
      y: 9,
      widgetType: "reconciliation-lag",
    },
    {
      h: 5,
      i: "stale-resource-status-count#1",
      title: "Stale resource status",
      w: 1,
      x: 0,
      y: 14,
      widgetType: "stale-resource-status-count",
    },
  ],
  lg: [],
  md: [],
  sm: [],
};
reliabilityLayout.lg = reliabilityLayout.xl;
reliabilityLayout.md = reliabilityLayout.xl;
reliabilityLayout.sm = reliabilityLayout.xl.map((item, index) => ({
  ...item,
  h: 5,
  w: 1,
  x: 0,
  y: index * 5,
}));

const textStyle = { fontSize: 11 };
function ReliabilitySummary({
  adminOnly = true,
  icon,
  showHeader = true,
  title = "Reliability summary",
}: {
  adminOnly?: boolean;
  icon?: React.ReactNode;
  showHeader?: boolean;
  title?: string;
}) {
  return (
    <DashboardWidget
      adminOnly={adminOnly}
      icon={icon}
      showHeader={showHeader}
      title={title}
    >
      <div
        style={{
          columnGap: 32,
          display: "grid",
          gridTemplateColumns: "repeat(2, minmax(0, 1fr))",
        }}
      >
        <DashboardSummaryList
          gap={12}
          items={[
            {
              label: "Request rate",
              trend: { direction: "up", tone: "positive" },
              value: "12.500 requests/sec",
            },
            {
              label: "Error rate",
              trend: { direction: "up", tone: "positive" },
              value: "1.250 %",
            },
            {
              label: "Median latency",
              trend: { direction: "up", tone: "positive" },
              value: "0.084 sec",
            },
          ]}
        />
        <DashboardSummaryList
          gap={8}
          items={[
            { label: "Reconciliation failures", value: "23 failures" },
            { label: "Reconciliation retries", value: "16 retries" },
            {
              label: "Reconciliation successes",
              trend: { direction: "up", tone: "positive" },
              value: "307 successes",
            },
            { label: "Reconciliation lag", value: "0.047 sec" },
            {
              label: "Stale resource status",
              trend: { direction: "up", tone: "positive" },
              value: "22 count",
            },
          ]}
        />
      </div>
    </DashboardWidget>
  );
}
function MetricWidget({
  adminOnly = true,
  icon,
  showHeader = true,
  title,
  value,
  description,
  children,
}: {
  adminOnly?: boolean;
  icon?: React.ReactNode;
  showHeader?: boolean;
  title: string;
  value: string;
  description: string;
  children?: React.ReactNode;
}) {
  return (
    <DashboardWidget
      adminOnly={adminOnly}
      icon={icon}
      showHeader={showHeader}
      title={title}
    >
      <div style={{ textAlign: "center" }}>
        <Title headingLevel="h3" size="md">
          {value}
        </Title>
        <Content component="small">(last 5 minutes)</Content>
      </div>
      {children ?? <DashboardSparkline caption="Last 24 hours" />}
      <Content
        component="small"
        style={{ display: "block", marginTop: 8, ...textStyle }}
      >
        {description}
      </Content>
    </DashboardWidget>
  );
}
function ReconciliationFailures({
  adminOnly = true,
  icon,
  showHeader = true,
  title = "Reconciliation failures",
}: {
  adminOnly?: boolean;
  icon?: React.ReactNode;
  showHeader?: boolean;
  title?: string;
}) {
  return (
    <DashboardWidget
      adminOnly={adminOnly}
      icon={icon}
      showHeader={showHeader}
      title={title}
    >
      <div
        style={{
          display: "grid",
          gap: 10,
          gridTemplateColumns: "1fr 1fr",
          ...textStyle,
        }}
      >
        <strong>Invalid Config</strong>
        <span>Missing Prerequisite &nbsp; 8</span>
        <strong>Identity Invalid</strong>
        <span>3</span>
      </div>
      <DashboardSparkline
        caption="Last 24 hours"
        data={[2, 8, 3, 11, 4, 8, 2, 5, 3, 7, 4, 6]}
      />
      <Content component="small" style={textStyle}>
        The current value is the number of failed reconciliation attempts during
        the last 24 hours.
      </Content>
    </DashboardWidget>
  );
}
function ReconciliationRetries({
  adminOnly = true,
  icon,
  showHeader = true,
  title = "Reconciliation retries",
}: {
  adminOnly?: boolean;
  icon?: React.ReactNode;
  showHeader?: boolean;
  title?: string;
}) {
  return (
    <DashboardWidget
      adminOnly={adminOnly}
      icon={icon}
      showHeader={showHeader}
      title={title}
    >
      <div style={{ textAlign: "center" }}>
        <Title headingLevel="h3" size="md">
          16 retries
        </Title>
        <Content component="small">(last 24 hours)</Content>
      </div>
      <div
        style={{
          display: "grid",
          gap: 8,
          gridTemplateColumns: "1fr 1fr",
          marginTop: 18,
          ...textStyle,
        }}
      >
        <strong>K8s Conflict</strong>
        <span>Grpc Unavailable &nbsp; 5</span>
        <strong>Keycloak Transient</strong>
        <span>Dependency Not Ready &nbsp; 2</span>
      </div>
      <DashboardSparkline
        caption="Last 24 hours"
        data={[1, 5, 2, 4, 1, 3, 2, 6, 2, 4]}
      />
    </DashboardWidget>
  );
}
export function ReliabilityDashboardPageSurface() {
  return (
    <>
      <PageSection data-page-region="header" hasBodyWrapper={false}>
        <Content>
          <Title headingLevel="h1" size="2xl">
            Reliability
          </Title>
          <p>
            API request rate, 5xx error rate, and median latency with 24-hour
            trends. Metrics refresh every 15 minutes.
          </p>
        </Content>
      </PageSection>
      <PageSection
        data-page-region="body"
        hasBodyWrapper={false}
        isFilled
        variant="secondary"
      >
        <div style={{ display: "grid", gap: 8 }}>
          <DashboardNavigation
            current="Reliability"
            showLayoutActions={false}
            updatedLabel="Updated 2 minutes ago"
          />
          <DashboardPurpose>
            This dashboard answers: Is the platform serving requests reliably?
            Are reconciliations failing, retrying, or falling behind? Are there
            incidents or alerts requiring investigation? These platform-wide
            operational signals are for system admins; production views should
            scope them to permitted gateways where supported.
          </DashboardPurpose>
          <DashboardFilters
            actions={
              <>
                <Button variant="link" onClick={() => undefined}>
                  Reset to default layout
                </Button>
                <Button
                  variant="secondary"
                  icon={<PlusCircleIcon />}
                  onClick={() => undefined}
                >
                  Add widgets
                </Button>
              </>
            }
            showRange={false}
          />
          <MockupWidgetizedLayout
            columns={{ xl: 3, lg: 3, md: 3, sm: 1 }}
            template={reliabilityLayout}
          >
            <ReliabilitySummary
              adminOnly
              icon={<ChartLineIcon />}
              title="Reliability summary"
            />
            <MetricWidget
              adminOnly
              icon={<TachometerAltIcon />}
              title="API request rate"
              value="12.500 requests/sec"
              description="The current API request rate is the average number of requests per second over the last 5 minutes. Each trend point is an hourly sample from the previous 24 hours."
            />
            <MetricWidget
              adminOnly
              icon={<ExclamationCircleIcon />}
              title="API error rate"
              value="1.250 %"
              description="The current API error rate is the percentage of API requests that returned an HTTP 5xx response over the last 5 minutes."
            />
            <MetricWidget
              adminOnly
              icon={<OutlinedClockIcon />}
              title="API latency"
              value="0.084 sec"
              description="The current API latency is the median response time for API requests over the last 5 minutes."
            />
            <ReconciliationFailures
              adminOnly
              icon={<ExclamationCircleIcon />}
              title="Reconciliation failures"
            />
            <ReconciliationRetries
              adminOnly
              icon={<SyncAltIcon />}
              title="Reconciliation retries"
            />
            <MetricWidget
              adminOnly
              icon={<OutlinedClockIcon />}
              title="Reconciliation lag"
              value="0.047 sec"
              description="The current reconciliation lag is the median time, in seconds, from when a reconciliation becomes eligible to when it completes."
            />
            <MetricWidget
              adminOnly
              icon={<CheckCircleIcon />}
              title="Stale resource status"
              value="22 count"
              description="The current value is the number of resources whose observed status has not caught up with their desired state."
            />
          </MockupWidgetizedLayout>
        </div>
      </PageSection>
    </>
  );
}

export function ReliabilityDashboardMockup() {
  return (
    <MockupShell>
      <ReliabilityDashboardPageSurface />
    </MockupShell>
  );
}
