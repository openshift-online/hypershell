import {
  Button,
  Content,
  PageSection,
  Title,
} from "@patternfly/react-core";
import { PlusCircleIcon } from "@patternfly/react-icons";
import {
  DashboardFilters,
  DashboardNavigation,
  DashboardPurpose,
} from "../common/dashboard-controls";
import { DashboardDonut, DashboardWidget } from "../common/dashboard-widget";
import { MockupShell } from "../shell/mockup-template";

const Row = ({ label, value }: { label: string; value: string }) => (
  <div
    style={{ display: "flex", fontSize: 12, justifyContent: "space-between" }}
  >
    <span>{label}</span>
    <strong>{value}</strong>
  </div>
);

export function PlatformInventoryDashboardPageSurface() {
  return (
    <>
      <PageSection data-page-region="header" hasBodyWrapper={false}>
        <Content>
          <Title headingLevel="h1" size="2xl">
            Platform inventory
          </Title>
          <p>
            Review the managed clusters available across the HyperShell
            platform.
          </p>
        </Content>
      </PageSection>
      <PageSection
        data-page-region="body"
        hasBodyWrapper={false}
        isFilled
        variant="secondary"
      >
        <DashboardNavigation current="Inventory" showLayoutActions={false} />
        <DashboardPurpose>
          How large is the managed cluster fleet, and where is it distributed?
          Use this system-wide view to understand provider and regional
          coverage.
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
        />
        <div
          style={{
            display: "grid",
            gap: 8,
            gridTemplateColumns: "repeat(4, minmax(0, 1fr))",
            gridTemplateRows: "280px",
          }}
        >
          <DashboardWidget adminOnly title="Inventory summary">
            <div style={{ display: "grid", gap: 18 }}>
              <Row label="Managed clusters" value="8" />
              <Row label="Recently added (30d)" value="2" />
              <Row label="Healthy clusters" value="7" />
              <Row label="Clusters needing attention" value="1" />
              <Content component="small">
                Counts represent the complete platform inventory.
              </Content>
            </div>
          </DashboardWidget>
          <DashboardWidget adminOnly title="Cluster providers">
            <DashboardDonut
              label="Providers"
              value="8"
              segments={[
                { label: "AWS", value: 4, color: "#0066cc" },
                { label: "IBM Cloud", value: 3, color: "#6753ac" },
                { label: "Other", value: 1, color: "#d2d2d2" },
              ]}
            />
          </DashboardWidget>
          <DashboardWidget
            adminOnly
            title="Cluster regions"
            style={{ gridColumn: "3 / span 2" }}
          >
            <DashboardDonut
              label="Regions"
              value="8"
              segments={[
                { label: "us-east", value: 3, color: "#0066cc" },
                { label: "us-central", value: 2, color: "#3e8635" },
                { label: "eu-west", value: 2, color: "#6753ac" },
                { label: "ap-south", value: 1, color: "#f0ab00" },
              ]}
            />
          </DashboardWidget>
        </div>
      </PageSection>
    </>
  );
}

export function PlatformInventoryDashboardMockup() {
  return (
    <MockupShell>
      <PlatformInventoryDashboardPageSurface />
    </MockupShell>
  );
}
