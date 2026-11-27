import {
  Alert,
  Button,
  Content,
  Label,
  PageSection,
  Title,
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
import { CostAboutEstimates, CostViewSwitch } from "./cost-dashboard";

/**
 * My cost view of the cost dashboard (WEB-COST-10). Every signed-in user sees
 * it; administrators reach it through the Platform / My cost switch. Widgets
 * carry no System label because they show only the user's own gateways.
 */

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

const charged = (
  <Label color="blue" isCompact>
    Charged to me
  </Label>
);

export function MyCostDashboardPageSurface({
  isAdmin = false,
}: {
  isAdmin?: boolean;
}) {
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
          What do the gateways I am the billing owner of cost to run, and what
          do the gateways shared with me cost? Each gateway is charged in full
          to its billing owner, the owner with the oldest owner grant.
        </DashboardPurpose>
        <DashboardFilters
          timeRanges={[7, 30]}
          actions={isAdmin ? <CostViewSwitch current="My cost" /> : undefined}
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
            marginTop: 8,
          }}
        >
          <DashboardWidget
            title="Cost charged to me"
            style={{ gridColumn: "1 / span 2" }}
          >
            <Title headingLevel="h3" size="lg">
              $740.40 / month
            </Title>
            <Content component="small">
              2 gateways where you are the billing owner
            </Content>
          </DashboardWidget>
          <DashboardWidget
            title="Shared with me"
            style={{ gridColumn: "3 / span 2" }}
          >
            <Title headingLevel="h3" size="lg">
              $287.50 / month
            </Title>
            <Content component="small">
              1 gateway charged to another billing owner
            </Content>
          </DashboardWidget>
          <DashboardWidget
            title="Daily cost charged to me"
            style={{ gridColumn: "1 / -1" }}
          >
            <DashboardSparkline
              caption="Daily cost, selected period"
              data={[23, 24, 24, 25, 24, 25, 25]}
            />
          </DashboardWidget>
          <DashboardWidget
            title="Cost by gateway"
            style={{ gridColumn: "1 / -1" }}
          >
            <Table
              columns={[
                "Gateway",
                "My role",
                "Billing owner",
                "Cluster",
                "Monthly estimate",
              ]}
              rows={[
                ["payments-agents", "Owner", charged, "Cluster alpha", "$612"],
                [
                  <>
                    docs-bot{" "}
                    <Label color="orange" isCompact>
                      Unused
                    </Label>
                  </>,
                  "Owner",
                  charged,
                  "Cluster beta",
                  "$128.40",
                ],
                ["team-research", "Viewer", "dana", "Cluster alpha", "$287.50"],
              ]}
            />
          </DashboardWidget>
          <DashboardWidget
            title="Cost by workspace"
            style={{ gridColumn: "1 / -1" }}
          >
            <Table
              columns={["Workspace", "Gateway", "Monthly estimate"]}
              rows={[
                ["checkout", "payments-agents", "$438.20"],
                ["refunds", "payments-agents", "$121.60"],
              ]}
            />
          </DashboardWidget>
        </div>
        <CostAboutEstimates />
      </PageSection>
    </>
  );
}
