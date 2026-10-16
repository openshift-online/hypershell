import {
  Alert,
  Button,
  Content,
  PageSection,
  Title,
} from "@patternfly/react-core";
import {
  DashboardFilters,
  DashboardNavigation,
  DashboardPurpose,
  futureDashboards,
} from "../common/dashboard-controls";
import {
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

export function CostDashboardPageSurface() {
  return (
    <>
      <PageSection data-page-region="header" hasBodyWrapper={false}>
        <Content>
          <Title headingLevel="h1" size="2xl">
            Cost dashboard
          </Title>
          <p>
            Directional operational estimates for capacity planning. This view
            is not billing-grade reporting.
          </p>
        </Content>
      </PageSection>
      <PageSection
        data-page-region="body"
        hasBodyWrapper={false}
        isFilled
        variant="secondary"
      >
        <DashboardNavigation current="Cost" destinations={futureDashboards} />
        <DashboardPurpose>
          This dashboard answers: What is the directional operating cost of the
          platform, where is it concentrated, and how much may be tied to idle
          resources? It is restricted to system admins and is not billing-grade
          reporting.
        </DashboardPurpose>
        <DashboardFilters />
        <Alert isInline title="Estimated costs only" variant="info">
          Model inputs and limitations are documented.{" "}
          <Button component="a" href="#cost-estimates" variant="link">
            About these estimates
          </Button>
        </Alert>
        <div
          style={{
            display: "grid",
            gap: 8,
            gridTemplateColumns: "repeat(4, minmax(0, 1fr))",
            gridTemplateRows: "230px 300px 220px",
          }}
        >
          <DashboardWidget
            adminOnly
            title="Cost summary"
            style={{ gridColumn: "1 / span 2" }}
          >
            <Title headingLevel="h3" size="lg">
              $18,420 / month
            </Title>
            <div style={{ display: "grid", gap: 14, marginTop: 22 }}>
              <Row label="Idle resource cost" value="$2,180" />
              <Row label="Cost per active user" value="$46.05" />
              <Row label="Cost per gateway" value="$189.90" />
              <Row label="Cost per sandbox" value="$54.18" />
            </div>
          </DashboardWidget>
          <DashboardWidget adminOnly title="Idle resource cost">
            <Title headingLevel="h3" size="lg">
              $2,180
            </Title>
            <Content component="small">12% of estimated monthly cost</Content>
            <DashboardSparkline
              data={[12, 15, 14, 18, 17, 20, 19, 23, 22, 21, 25, 27]}
              caption="Last 30 days"
            />
          </DashboardWidget>
          <DashboardWidget adminOnly title="Cost per active user">
            <Title headingLevel="h3" size="lg">
              $46.05
            </Title>
            <Content component="small">
              Directional estimate for selected period
            </Content>
            <DashboardSparkline
              data={[35, 39, 38, 42, 40, 43, 44, 45, 46, 47, 46, 46]}
              caption="Last 30 days"
            />
          </DashboardWidget>
          <DashboardWidget
            adminOnly
            title="By environment"
            style={{ gridColumn: "1 / span 2" }}
          >
            <Content style={{ fontSize: 12 }}>Estimated monthly cost</Content>
            <DashboardSparkline
              data={[20, 23, 22, 28, 30, 29, 34, 37, 35, 39, 42, 45]}
              caption="Development · staging · production"
            />
            <Row label="Production" value="$11,240" />
            <Row label="Staging" value="$4,810" />
            <Row label="Development" value="$2,370" />
          </DashboardWidget>
          <DashboardWidget adminOnly title="By fleet">
            <Content style={{ fontSize: 12 }}>Estimated monthly cost</Content>
            <DashboardSparkline
              data={[18, 22, 25, 24, 31, 30, 34, 36, 41, 39, 44, 48]}
              caption="Top fleets · selected period"
            />
            <Row label="Fleet alpha" value="$8,420" />
            <Row label="Fleet beta" value="$5,180" />
          </DashboardWidget>
          <DashboardWidget adminOnly title="By resource class">
            <Content style={{ fontSize: 12 }}>Estimated monthly cost</Content>
            <DashboardSparkline
              data={[15, 18, 17, 20, 22, 25, 24, 27, 29, 31, 30, 34]}
              caption="Gateways · sandboxes · clusters"
            />
            <Row label="Gateways" value="$9,820" />
            <Row label="Sandboxes" value="$6,440" />
          </DashboardWidget>
        </div>
      </PageSection>
    </>
  );
}

export function CostDashboardMockup() {
  return (
    <MockupShell>
      <CostDashboardPageSurface />
    </MockupShell>
  );
}
