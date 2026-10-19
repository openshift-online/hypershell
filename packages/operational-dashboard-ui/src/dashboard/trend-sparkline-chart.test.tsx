import { render } from "@testing-library/react";
import type { ReactElement } from "react";
import { describe, expect, it, vi } from "vitest";
import { IntlProvider } from "react-intl";

import { TrendSparklineChart } from "./trend-sparkline-chart";

const chartMocks = vi.hoisted(() => ({
  areaData: [] as unknown[],
  labels: undefined as
    ((props: { datum: { y: number | null } }) => string) | undefined,
}));

vi.mock("../patternfly/victory-charts", () => ({
  ChartArea: ({ data }: { data: unknown[] }) => {
    chartMocks.areaData = data;
    return <path aria-hidden="true" />;
  },
  ChartGroup: ({
    children,
    containerComponent,
  }: {
    children: ReactElement;
    containerComponent: { props: { labels?: typeof chartMocks.labels } };
  }) => {
    chartMocks.labels = containerComponent.props.labels;
    return <svg>{children}</svg>;
  },
  ChartThemeColor: { blue: "blue" },
  ChartVoronoiContainer: ({
    labels,
  }: {
    labels: (props: { datum: { y: number | null } }) => string;
  }) => {
    chartMocks.labels = labels;
    return null;
  },
}));

describe("TrendSparklineChart", () => {
  it("preserves missing values but suppresses their tooltips", () => {
    const title = "Gateway request volume";

    render(
      <IntlProvider locale="en">
        <TrendSparklineChart
          title={title}
          trend={{
            points: [
              { label: "2026-10-14", value: 100 },
              { label: "2026-10-15", value: null },
              { label: "2026-10-16", value: 120 },
            ],
          }}
        />
      </IntlProvider>,
    );

    expect(chartMocks.areaData).toEqual([
      { name: title, x: "2026-10-14", y: 100 },
      { name: title, x: "2026-10-15", y: null },
      { name: title, x: "2026-10-16", y: 120 },
    ]);
    expect(chartMocks.labels?.({ datum: { y: null } })).toBe("");
  });
});
