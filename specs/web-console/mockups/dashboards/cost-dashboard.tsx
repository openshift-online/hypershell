import {
  Alert,
  Button,
  Card,
  CardBody,
  CardHeader,
  CardTitle,
  Content,
  Label,
  PageSection,
  Title,
  ToggleGroup,
  ToggleGroupItem,
} from "@patternfly/react-core";
import {
  Table as PfTable,
  Tbody,
  Td,
  Th,
  Thead,
  Tr,
} from "@patternfly/react-table";
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

const Row = ({ label, value }: { label: string; value: string }) => (
  <div
    style={{ display: "flex", fontSize: 12, justifyContent: "space-between" }}
  >
    <span>{label}</span>
    <strong>{value}</strong>
  </div>
);

const Table = ({
  columns,
  rows,
}: {
  columns: readonly string[];
  rows: readonly (readonly React.ReactNode[])[];
}) => (
  <PfTable aria-label={columns.join(", ")} variant="compact">
    <Thead>
      <Tr>
        {columns.map((column) => (
          <Th key={column}>{column}</Th>
        ))}
      </Tr>
    </Thead>
    <Tbody>
      {rows.map((row, index) => (
        <Tr key={index}>
          {row.map((cell, cellIndex) => (
            <Td dataLabel={columns[cellIndex]} key={cellIndex}>
              {cell}
            </Td>
          ))}
        </Tr>
      ))}
    </Tbody>
  </PfTable>
);

/** View switch shown to administrators only (WEB-COST-02). */
export function CostViewSwitch({
  current,
}: {
  current: "Platform" | "My cost";
}) {
  return (
    <ToggleGroup aria-label="Cost view">
      <ToggleGroupItem
        buttonId="cost-view-platform"
        isSelected={current === "Platform"}
        text="Platform"
      />
      <ToggleGroupItem
        buttonId="cost-view-mine"
        isSelected={current === "My cost"}
        text="My cost"
      />
    </ToggleGroup>
  );
}

/** In-page model explanation (WEB-COST-07). */
export function CostAboutEstimates() {
  return (
    <Card id="cost-estimates" isCompact style={{ marginTop: 8 }}>
      <CardHeader>
        <CardTitle component="h2">About these estimates</CardTitle>
      </CardHeader>
      <CardBody>
        <Content>
          <p>
            Cost is estimated from reserved capacity: CPU and memory requests,
            and node capacity that nothing reserves, multiplied by a rate card
            supplied by this deployment (version 2026-11-26, USD). Monthly
            figures use a 730-hour month.
          </p>
          <p>
            Each gateway, including all its workspaces and sandboxes, is charged
            to one billing owner: the current owner with the oldest owner grant.
            If that owner leaves, the gateway&apos;s cost in the selected period
            moves to the next-oldest owner.
          </p>
          <p>
            Not included: storage, network egress, load balancers, managed
            services such as databases and secrets, and LLM spend.
          </p>
          <p>
            These values are directional estimates for capacity planning. They
            are not billing-grade and are not reconciled with a cloud invoice.
          </p>
        </Content>
      </CardBody>
    </Card>
  );
}

export function CostDashboardPageSurface() {
  return (
    <>
      <PageSection
        data-page-region="header"
        hasBodyWrapper={false}
        variant="default"
      >
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
        <DashboardFilters
          timeRanges={[7, 30]}
          actions={<CostViewSwitch current="Platform" />}
        />
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
            gridTemplateRows: "230px 300px auto",
            marginTop: 8,
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
            <Row label="Unused gateways" value="$410" />
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
          <DashboardWidget adminOnly title="By cluster">
            <Content style={{ fontSize: 12 }}>Estimated monthly cost</Content>
            <DashboardSparkline
              data={[18, 22, 25, 24, 31, 30, 34, 36, 41, 39, 44, 48]}
              caption="Top clusters · selected period"
            />
            <Row label="Cluster alpha" value="$8,420" />
            <Row label="Cluster beta" value="$5,180" />
          </DashboardWidget>
          <DashboardWidget adminOnly title="By resource class">
            <Content style={{ fontSize: 12 }}>Estimated monthly cost</Content>
            <DashboardSparkline
              data={[15, 18, 17, 20, 22, 25, 24, 27, 29, 31, 30, 34]}
              caption="Gateways · sandboxes · clusters"
            />
            <Row label="Gateways" value="$9,820" />
            <Row label="Sandboxes" value="$6,440" />
            <Row label="Platform overhead" value="$0" />
            <Row label="Idle capacity" value="$2,180" />
          </DashboardWidget>
          <DashboardWidget
            adminOnly
            title="Most expensive gateways"
            style={{ gridColumn: "1 / span 2" }}
          >
            <Table
              columns={[
                "Gateway",
                "Cluster",
                "Billing owner",
                "Monthly estimate",
              ]}
              rows={[
                ["payments-agents", "Cluster alpha", "alice", "$612"],
                [
                  <>
                    ci-sandbox{" "}
                    <Label color="orange" isCompact>
                      Unused
                    </Label>
                  </>,
                  "Cluster beta",
                  "bob",
                  "$410",
                ],
              ]}
            />
          </DashboardWidget>
          <DashboardWidget
            adminOnly
            title="By billing owner"
            style={{ gridColumn: "3 / span 2" }}
          >
            <Table
              columns={["Billing owner", "Gateways", "Monthly estimate"]}
              rows={[
                ["alice", "6", "$1,320"],
                ["bob", "4", "$860"],
                ["carol", "3", "$540"],
              ]}
            />
          </DashboardWidget>
        </div>
        <CostAboutEstimates />
      </PageSection>
    </>
  );
}
