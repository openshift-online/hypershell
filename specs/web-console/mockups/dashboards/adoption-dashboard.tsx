import { Content, Divider, PageSection, Title } from "@patternfly/react-core";
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

const Row = ({ label, value }: { label: string; value: string }) => (
  <div
    style={{ display: "flex", fontSize: 12, justifyContent: "space-between" }}
  >
    <span>{label}</span>
    <strong>{value}</strong>
  </div>
);

export function AdoptionDashboardPageSurface() {
  return (
    <>
      <PageSection
        data-page-region="header"
        hasBodyWrapper={false}
        variant="default"
      >
        <Content>
          <Title headingLevel="h1" size="2xl">
            Adoption dashboard
          </Title>
          <p>
            Understand who is using HyperShell, how adoption is changing, and
            where users drop off.
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
          current="Adoption"
          destinations={futureDashboards}
        />
        <DashboardPurpose>
          This dashboard answers: Are people adopting HyperShell? Are
          registrations, active use, conversion, and retention moving in the
          right direction? These are aggregate measures; system-wide views are
          for platform admins, while fleet-scoped views follow the viewer's
          access.
        </DashboardPurpose>
        <DashboardFilters />
        <div
          style={{
            display: "grid",
            gap: 8,
            gridTemplateColumns: "repeat(3, minmax(0, 1fr))",
            gridTemplateRows: "260px 330px",
          }}
        >
          <DashboardWidget
            adminOnly
            title="Adoption summary"
            style={{ gridColumn: "1 / span 2" }}
          >
            <div style={{ display: "grid", gap: 16 }}>
              <Title headingLevel="h3" size="lg">
                450 registered users
              </Title>
              <Row label="New registrations (selected period)" value="48" />
              <Row label="Daily active users" value="186" />
              <Row label="Weekly active users" value="312" />
              <Row label="Monthly active users" value="398" />
              <Row label="Inactive users (30+ days)" value="52" />
              <Row label="7-day retention" value="78%" />
              <Row label="30-day retention" value="-" />
              <Divider />
              <Content component="small">
                Active users are counted when they perform a qualifying
                HyperShell activity. Aggregate-only view; no user-level data is
                displayed.
              </Content>
            </div>
          </DashboardWidget>
          <DashboardWidget adminOnly title="Registration to first resource">
            <DashboardDonut
              label="Conversion"
              value="68%"
              segments={[
                { label: "First gateway", value: 68, color: "#0066cc" },
                { label: "Not converted", value: 32, color: "#d2d2d2" },
              ]}
            />
            <Row label="First sandbox" value="61%" />
            <Row label="Median time to first resource" value="2h 14m" />
          </DashboardWidget>
          <DashboardWidget
            adminOnly
            title="Adoption trends"
            style={{ gridColumn: "1 / span 3" }}
          >
            <Title headingLevel="h4" size="md">
              New registrations
            </Title>
            <DashboardSparkline
              data={[18, 23, 27, 32, 31, 39, 44, 48, 55, 61, 67, 74]}
              caption="Last 30 days · daily bucket · UTC"
            />
            <Title headingLevel="h4" size="md">
              Active users
            </Title>
            <DashboardSparkline
              data={[12, 15, 18, 24, 29, 33, 36, 41, 47, 52, 58, 64]}
              caption="Last 30 days · daily bucket · UTC"
            />
          </DashboardWidget>
        </div>
      </PageSection>
    </>
  );
}
