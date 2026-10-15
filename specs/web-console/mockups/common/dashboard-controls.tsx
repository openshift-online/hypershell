import {
  Button,
  Flex,
  FlexItem,
  FormSelect,
  FormSelectOption,
  Label,
  Nav,
  NavItem,
  NavList,
} from "@patternfly/react-core";
import { PlusCircleIcon, SyncAltIcon } from "@patternfly/react-icons";

type DashboardDestination = readonly [label: string, adminOnly: boolean];

const currentDashboards: readonly DashboardDestination[] = [
  ["Overview", false],
  ["Provisioning", true],
  ["Inventory", true],
  ["Reliability", true],
];

export const futureDashboards: readonly DashboardDestination[] = [
  ["Overview", false],
  ["Adoption", true],
  ["Provisioning", true],
  ["Inventory", true],
  ["Reliability", true],
  ["Usage", false],
  ["Cost", true],
] as const;

export function DashboardPurpose({ children }: { children: React.ReactNode }) {
  return (
    <div
      style={{
        background: "#fff",
        borderLeft: "4px solid #0066cc",
        fontSize: 13,
        padding: "8px 16px",
      }}
    >
      <strong>What this dashboard answers</strong>
      <div style={{ marginTop: 4 }}>{children}</div>
    </div>
  );
}

export function DashboardNavigation({
  current,
  destinations = currentDashboards,
  isAdmin = true,
  showLayoutActions = true,
  updatedLabel,
}: {
  current: string;
  destinations?: readonly DashboardDestination[];
  isAdmin?: boolean;
  showLayoutActions?: boolean;
  updatedLabel?: string;
}) {
  const routes: Record<string, string> = {
    Adoption: "/dashboard/adoption",
    Cost: "/dashboard/cost",
    Inventory: "/dashboard/inventory",
    Overview: "/dashboard",
    Provisioning: "/dashboard/provisioning",
    Reliability: "/dashboard/reliability",
    Usage: "/dashboard/usage",
  };

  return (
    <Flex
      alignItems={{ default: "alignItemsCenter" }}
      justifyContent={{ default: "justifyContentSpaceBetween" }}
      gap={{ default: "gapMd" }}
      style={{ flexWrap: "wrap" }}
    >
      <Nav aria-label="Dashboard" variant="horizontal">
        <NavList>
          {destinations
            .filter(([, adminOnly]) => !adminOnly || isAdmin)
            .map(([dashboard]) => (
              <NavItem
                key={dashboard}
                component="a"
                href={routes[dashboard]}
                isActive={dashboard === current}
              >
                {dashboard}
              </NavItem>
            ))}
        </NavList>
      </Nav>
      <Flex
        alignItems={{ default: "alignItemsCenter" }}
        gap={{ default: "gapSm" }}
      >
        <FlexItem>
          <Button
            aria-label="Refresh dashboard metrics"
            icon={<SyncAltIcon />}
            title="Refresh dashboard metrics"
            variant="plain"
          />
        </FlexItem>
        {updatedLabel ? (
          <FlexItem>
            <Label color="blue">{updatedLabel}</Label>
          </FlexItem>
        ) : null}
        {showLayoutActions ? (
          <FlexItem>
            <Button variant="link" onClick={() => undefined}>
              Reset to default layout
            </Button>
          </FlexItem>
        ) : null}
        {showLayoutActions ? (
          <FlexItem>
            <Button
              variant="secondary"
              icon={<PlusCircleIcon />}
              onClick={() => undefined}
            >
              Add widgets
            </Button>
          </FlexItem>
        ) : null}
      </Flex>
    </Flex>
  );
}

export function DashboardFilters({
  actions,
  dimensions = [],
  onTimeRangeChange,
  showRange = true,
  timeRanges = [7, 30, 90],
  timeRange = 7,
}: {
  actions?: React.ReactNode;
  dimensions?: string[];
  onTimeRangeChange?: (timeRange: number) => void;
  showRange?: boolean;
  timeRanges?: readonly number[];
  timeRange?: number;
}) {
  return (
    <Flex
      alignItems={{ default: "alignItemsCenter" }}
      gap={{ default: "gapSm" }}
      justifyContent={{ default: "justifyContentSpaceBetween" }}
      style={{ flexWrap: "wrap", margin: "14px 0 18px" }}
    >
      <Flex
        alignItems={{ default: "alignItemsCenter" }}
        gap={{ default: "gapSm" }}
      >
        {showRange ? (
          <FlexItem>
            <FormSelect
              aria-label="Time range"
              ouiaId="time-range"
              onChange={(event) =>
                onTimeRangeChange?.(Number(event.currentTarget.value))
              }
              style={{ minWidth: 130 }}
              value={String(timeRange)}
            >
              {timeRanges.map((days) => (
                <FormSelectOption
                  key={days}
                  value={String(days)}
                  label={`Last ${String(days)} days`}
                />
              ))}
            </FormSelect>
          </FlexItem>
        ) : null}
        {dimensions.map((dimension) => (
          <FlexItem key={dimension}>
            <FormSelect
              aria-label={dimension}
              ouiaId={`filter-${dimension.toLowerCase()}`}
              style={{ minWidth: 145 }}
            >
              <FormSelectOption value="all" label={`${dimension}: All`} />
              <FormSelectOption value="one" label={`${dimension}: Example`} />
            </FormSelect>
          </FlexItem>
        ))}
      </Flex>
      {actions ? (
        <Flex
          alignItems={{ default: "alignItemsCenter" }}
          gap={{ default: "gapSm" }}
        >
          {actions}
        </Flex>
      ) : null}
    </Flex>
  );
}
