import { ChartDonut } from "@patternfly/react-charts/victory";
import { useEffect, useRef, useState } from "react";

import type {
  StatusDonutDatum,
  StatusDonutLegendDatum,
} from "./status-donut-data";
import "../pages/dashboard-widget.css";

/** Rendered pixel size; must match the donut wrapper and ChartDonut height. */
export const STATUS_DONUT_CHART_HEIGHT = 165;

/** Compact size for standard metric widgets without trend sparklines. */
export const COMPACT_STATUS_DONUT_CHART_HEIGHT = 130;

/** Extra right padding keeps the legend inside the clipped widget body. */
export const STATUS_DONUT_CHART_PADDING = {
  bottom: 25,
  left: 20,
  right: 145,
  top: 20,
} as const;

export const COMPACT_STATUS_DONUT_CHART_PADDING = {
  bottom: 12,
  left: 12,
  right: 100,
  top: 4,
} as const;

/** Extra bottom padding reserves space for the capacity subtitle without enlarging the donut. */
export const COMPACT_STATUS_DONUT_WITH_SUBTITLE_CHART_PADDING = {
  bottom: 28,
  left: 12,
  right: 115,
  top: 4,
} as const;

/** Same donut ring as compact; extra height fits the subtitle below the chart. */
export const COMPACT_STATUS_DONUT_WITH_SUBTITLE_CHART_HEIGHT =
  COMPACT_STATUS_DONUT_CHART_HEIGHT -
  COMPACT_STATUS_DONUT_CHART_PADDING.top -
  COMPACT_STATUS_DONUT_CHART_PADDING.bottom +
  COMPACT_STATUS_DONUT_WITH_SUBTITLE_CHART_PADDING.top +
  COMPACT_STATUS_DONUT_WITH_SUBTITLE_CHART_PADDING.bottom;

export type StatusDonutChartSize = "compact" | "default";

export interface StatusDonutChartProps {
  ariaDesc: string;
  ariaTitle: string;
  colorScale: readonly string[];
  data: readonly StatusDonutDatum[];
  dataLabel: (datum: StatusDonutDatum) => string | null;
  legendData: readonly StatusDonutLegendDatum[];
  size?: StatusDonutChartSize;
  subTitle?: string;
  title: string;
}

export function StatusDonutChart({
  ariaDesc,
  ariaTitle,
  colorScale,
  data,
  dataLabel,
  legendData,
  size = "default",
  subTitle,
  title,
}: Readonly<StatusDonutChartProps>) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(275);
  const compactWithSubtitle = size === "compact" && subTitle !== undefined;
  const chartHeight =
    size === "compact"
      ? compactWithSubtitle
        ? COMPACT_STATUS_DONUT_WITH_SUBTITLE_CHART_HEIGHT
        : COMPACT_STATUS_DONUT_CHART_HEIGHT
      : STATUS_DONUT_CHART_HEIGHT;
  const chartPadding =
    size === "compact"
      ? compactWithSubtitle
        ? COMPACT_STATUS_DONUT_WITH_SUBTITLE_CHART_PADDING
        : COMPACT_STATUS_DONUT_CHART_PADDING
      : STATUS_DONUT_CHART_PADDING;

  useEffect(() => {
    const node = containerRef.current;
    if (!node) {
      return;
    }

    const observer = new ResizeObserver((entries) => {
      const nextWidth = entries[0]?.contentRect.width;
      if (nextWidth && nextWidth > 0) {
        setWidth(nextWidth);
      }
    });
    observer.observe(node);

    return () => {
      observer.disconnect();
    };
  }, []);

  if (data.length === 0) {
    return null;
  }

  return (
    <div
      ref={containerRef}
      className={
        size === "compact"
          ? "hypershell-dashboard-status-donut-chart hypershell-dashboard-status-donut-chart--compact"
          : "hypershell-dashboard-status-donut-chart"
      }
      style={{ height: chartHeight, width: "100%" }}
    >
      <ChartDonut
        ariaDesc={ariaDesc}
        ariaTitle={ariaTitle}
        colorScale={[...colorScale]}
        constrainToVisibleArea
        data={[...data]}
        height={chartHeight}
        labels={({ datum }: { datum: StatusDonutDatum }) => dataLabel(datum)}
        legendData={[...legendData]}
        legendOrientation="vertical"
        legendPosition="right"
        padding={chartPadding}
        subTitle={subTitle}
        subTitlePosition={subTitle ? "bottom" : undefined}
        title={title}
        width={width}
      />
    </div>
  );
}
