import { Content, Label, PageSection, Title } from "@patternfly/react-core";
import {
  ChartLineIcon,
  ContainerNodeIcon,
  CubesIcon,
  ExclamationCircleIcon,
  HourglassHalfIcon,
  OutlinedClockIcon,
} from "@patternfly/react-icons";
import type { ExtendedTemplateConfig } from "@patternfly/widgetized-dashboard";
import {
  DashboardFilters,
  DashboardNavigation,
  DashboardPurpose,
  futureDashboards,
} from "../common/dashboard-controls";
import {
  DashboardDonut,
  DashboardSparkline,
  DashboardWidget,
  MockupWidgetizedLayout,
} from "../common/dashboard-widget";
import { MockupShell } from "../shell/mockup-template";

const Row = ({ label, value }: { label: string; value: string }) => (
  <div
    style={{ display: "flex", fontSize: 12, justifyContent: "space-between" }}
  >
    <span>{label}</span>
    <strong>{value}</strong>
  </div>
);
function UsageCard({
  adminOnly,
  icon,
  title,
  value,
  children,
}: {
  adminOnly: boolean;
  icon?: React.ReactNode;
  title: string;
  value: string;
  children?: React.ReactNode;
}) {
  return (
    <DashboardWidget adminOnly={adminOnly} icon={icon} title={title}>
      <Title headingLevel="h3" size="lg">
        {value}
      </Title>
      {children ?? <DashboardSparkline caption="Last 30 days" />}
    </DashboardWidget>
  );
}

function createUsageLayout(): ExtendedTemplateConfig {
  const desktop = [
    {
      h: 5,
      i: "request-volume",
      title: "Gateway request volume",
      w: 1,
      widgetType: "request-volume",
      x: 0,
      y: 0,
    },
    {
      h: 5,
      i: "error-rate",
      title: "Gateway error rate",
      w: 1,
      widgetType: "error-rate",
      x: 1,
      y: 0,
    },
    {
      h: 5,
      i: "active-gateways",
      title: "Active gateways",
      w: 1,
      widgetType: "active-gateways",
      x: 2,
      y: 0,
    },
    {
      h: 5,
      i: "unused-gateways",
      title: "Provisioned but unused",
      w: 1,
      widgetType: "unused-gateways",
      x: 3,
      y: 0,
    },
    {
      h: 6,
      i: "sandbox-utilization",
      title: "Sandbox utilization",
      w: 2,
      widgetType: "sandbox-utilization",
      x: 0,
      y: 5,
    },
    {
      h: 6,
      i: "sandbox-lifetime",
      title: "Sandbox lifetime",
      w: 2,
      widgetType: "sandbox-lifetime",
      x: 2,
      y: 5,
    },
  ] as const;
  let stackedY = 0;
  const stacked = desktop.map((item) => {
    const positioned = { ...item, x: 0, y: stackedY };
    stackedY += item.h;
    return positioned;
  });

  return { xl: [...desktop], lg: [...desktop], md: [...desktop], sm: stacked };
}

const usageLayout = createUsageLayout();

export function UsageDashboardPageSurface({
  isAdmin = true,
}: Readonly<{ isAdmin?: boolean }>) {
  return (
    <>
      <PageSection data-page-region="header" hasBodyWrapper={false}>
        <Content>
          <Title headingLevel="h1" size="2xl">
            Usage dashboard
          </Title>
          <p>
            See how gateways and sandboxes are being used, not just how many
            have been provisioned.
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
          current="Usage"
          destinations={futureDashboards}
          isAdmin={isAdmin}
          showLayoutActions={false}
          updatedLabel="Updated 2 minutes ago"
        />
        <DashboardPurpose>
          This dashboard answers: Are my gateways receiving traffic? Which
          provisioned gateways are unused? How are my sandboxes being utilized,
          and how long do they live?{" "}
          {isAdmin
            ? "Metrics cover the entire fleet."
            : "Metrics are scoped to the resources you can access."}
        </DashboardPurpose>
        <DashboardFilters timeRanges={[7, 30]} />
        <MockupWidgetizedLayout template={usageLayout}>
          <UsageCard
            adminOnly={isAdmin}
            icon={<ChartLineIcon />}
            title="Gateway request volume"
            value="1.24M requests"
          >
            <DashboardSparkline
              data={[42, 49, 52, 58, 55, 61, 67, 70, 75, 79, 84, 91]}
              caption="Last 30 days - daily total"
            />
          </UsageCard>
          <UsageCard
            adminOnly={isAdmin}
            icon={<ExclamationCircleIcon />}
            title="Gateway error rate"
            value="1.25%"
          >
            <DashboardSparkline
              data={[8, 7, 9, 6, 7, 5, 6, 4, 5, 3, 4, 2]}
              caption="Last 30 days - daily rate"
            />
          </UsageCard>
          <UsageCard
            adminOnly={isAdmin}
            icon={<ContainerNodeIcon />}
            title="Active gateways"
            value="72"
          >
            <Row label="Provisioned gateways" value="97" />
            <Row label="Active share" value="74%" />
          </UsageCard>
          <UsageCard
            adminOnly={isAdmin}
            icon={<HourglassHalfIcon />}
            title="Provisioned but unused"
            value="25"
          >
            <Label color="orange">Review idle resources</Label>
            <Content
              component="small"
              style={{ display: "block", marginTop: 16 }}
            >
              Gateways with no qualifying request activity in the selected
              period.
            </Content>
          </UsageCard>
          <DashboardWidget
            adminOnly={isAdmin}
            icon={<CubesIcon />}
            title="Sandbox utilization"
          >
            <DashboardDonut
              label="Utilization"
              value="63%"
              segments={[
                { label: "In use", value: 63, color: "#0066cc" },
                { label: "Available", value: 37, color: "#d2d2d2" },
              ]}
            />
            <Row label="Active sandboxes" value="214" />
            <Row label="Provisioned sandboxes" value="340" />
          </DashboardWidget>
          <DashboardWidget
            adminOnly={isAdmin}
            icon={<OutlinedClockIcon />}
            title="Sandbox lifetime"
          >
            <Content style={{ fontSize: 12 }}>Average sandbox lifetime</Content>
            <Title headingLevel="h3" size="lg">
              18h 42m
            </Title>
            <DashboardSparkline
              data={[20, 24, 23, 27, 25, 30, 28, 32, 34, 33, 36, 39]}
              caption="Last 30 days"
            />
            <Row label="P50 lifetime" value="8h 12m" />
            <Row label="P95 lifetime" value="4d 6h" />
          </DashboardWidget>
        </MockupWidgetizedLayout>
      </PageSection>
    </>
  );
}

export function UsageDashboardMockup({
  isAdmin = true,
}: Readonly<{ isAdmin?: boolean }>) {
  return (
    <MockupShell>
      <UsageDashboardPageSurface isAdmin={isAdmin} />
    </MockupShell>
  );
}
