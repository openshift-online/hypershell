import { Content, Label, PageSection, Title } from "@patternfly/react-core";
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
  title,
  value,
  children,
}: {
  title: string;
  value: string;
  children?: React.ReactNode;
}) {
  return (
    <DashboardWidget title={title}>
      <Title headingLevel="h3" size="lg">
        {value}
      </Title>
      {children ?? <DashboardSparkline caption="Last 30 days" />}
    </DashboardWidget>
  );
}

export function UsageDashboardPageSurface() {
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
        <DashboardNavigation current="Usage" destinations={futureDashboards} />
        <DashboardPurpose>
          This dashboard answers: Are my gateways receiving traffic? Which
          provisioned gateways are unused? How are my sandboxes being utilized,
          and how long do they live? Metrics are scoped to the fleets,
          environments, and releases the viewer can access.
        </DashboardPurpose>
        <DashboardFilters dimensions={["Gateway release"]} />
        <div
          style={{
            display: "grid",
            gap: 8,
            gridTemplateColumns: "repeat(4, minmax(0, 1fr))",
            gridTemplateRows: "230px 300px 220px",
          }}
        >
          <UsageCard title="Gateway request volume" value="1.24M requests">
            <DashboardSparkline
              data={[42, 49, 52, 58, 55, 61, 67, 70, 75, 79, 84, 91]}
              caption="Last 30 days · daily total"
            />
          </UsageCard>
          <UsageCard title="Gateway error rate" value="1.25%">
            <DashboardSparkline
              data={[8, 7, 9, 6, 7, 5, 6, 4, 5, 3, 4, 2]}
              caption="Last 30 days · daily rate"
            />
          </UsageCard>
          <UsageCard title="Active gateways" value="72">
            <Row label="Provisioned gateways" value="97" />
            <Row label="Active share" value="74%" />
          </UsageCard>
          <UsageCard title="Provisioned but unused" value="25">
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
            title="Sandbox utilization"
            style={{ gridColumn: "1 / span 2" }}
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
            title="Sandbox lifetime"
            style={{ gridColumn: "3 / span 2" }}
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
        </div>
      </PageSection>
    </>
  );
}

export function UsageDashboardMockup() {
  return (
    <MockupShell>
      <UsageDashboardPageSurface />
    </MockupShell>
  );
}
