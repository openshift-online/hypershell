import {
  DescriptionList,
  DescriptionListDescription,
  DescriptionListGroup,
  DescriptionListTerm,
  Flex,
  FlexItem,
  Stack,
  StackItem,
  Title,
} from "@patternfly/react-core";
import type { ReactNode } from "react";

import type { OperationalMetricTrend } from "../application/dashboard-types";
import { TrendSparklineChart } from "./trend-sparkline-chart";
import "./dashboard-stat-panel.css";

export interface DashboardStatPanelRow {
  id: string;
  label: ReactNode;
  value: ReactNode;
}

export interface DashboardStatPanelSparkline {
  plotHeight?: number;
  title: string;
  tooltipLabel?: string;
  trend: OperationalMetricTrend;
}

const HORIZONTAL_TERM_WIDTH = {
  default: "22ch",
} as const;

export function DashboardStatPanel({
  ariaLabel,
  columns = "single",
  footer,
  heading,
  rows,
  sparkline,
}: Readonly<{
  ariaLabel: string;
  columns?: "single" | "two";
  footer?: ReactNode;
  heading?: ReactNode;
  rows: readonly DashboardStatPanelRow[];
  sparkline?: DashboardStatPanelSparkline;
}>) {
  const listClassName =
    columns === "two"
      ? "hypershell-dashboard-stat-panel__list hypershell-dashboard-stat-panel__list--two"
      : "hypershell-dashboard-stat-panel__list hypershell-dashboard-stat-panel__list--single";

  return (
    <Stack hasGutter className="hypershell-dashboard-stat-panel">
      {heading ? (
        <StackItem>
          <Flex justifyContent={{ default: "justifyContentCenter" }}>
            <FlexItem>{heading}</FlexItem>
          </Flex>
        </StackItem>
      ) : null}
      <StackItem>
        <DescriptionList
          aria-label={ariaLabel}
          className={listClassName}
          columnModifier={columns === "two" ? { default: "2Col" } : undefined}
          horizontalTermWidthModifier={HORIZONTAL_TERM_WIDTH}
          isCompact
          isFillColumns={columns === "two"}
          isFluid
          isHorizontal
        >
          {rows.map((row) => (
            <DescriptionListGroup key={row.id}>
              <DescriptionListTerm>{row.label}</DescriptionListTerm>
              <DescriptionListDescription>
                {row.value}
              </DescriptionListDescription>
            </DescriptionListGroup>
          ))}
        </DescriptionList>
      </StackItem>
      {sparkline ? (
        <StackItem>
          <div className="hypershell-dashboard-stat-panel__sparkline">
            <Title
              className="hypershell-dashboard-stat-panel__sparkline-title"
              headingLevel="h4"
              size="md"
            >
              {sparkline.title}
            </Title>
            <TrendSparklineChart
              plotHeight={sparkline.plotHeight}
              trend={sparkline.trend}
              title={sparkline.title}
              tooltipLabel={sparkline.tooltipLabel}
            />
          </div>
        </StackItem>
      ) : null}
      {footer ? <StackItem>{footer}</StackItem> : null}
    </Stack>
  );
}
