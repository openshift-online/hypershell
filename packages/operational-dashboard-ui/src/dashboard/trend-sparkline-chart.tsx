import {
  ChartArea,
  ChartGroup,
  ChartThemeColor,
  ChartVoronoiContainer,
} from "../patternfly/victory-charts";
import { useEffect, useRef, useState } from "react";
import { useIntl } from "react-intl";

import type { OperationalMetricTrend } from "../application/dashboard-types";
import { messages } from "../messages";
import "../pages/dashboard-widget.css";

interface SparklineDatum {
  name: string;
  x: string;
  y: number;
}

const SPARKLINE_PLOT_HEIGHT = 36;

export function TrendSparklineChart({
  trend,
  title,
}: Readonly<{
  trend: OperationalMetricTrend;
  title: string;
}>) {
  const intl = useIntl();
  const containerRef = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(220);

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

  if (trend.points.length < 2) {
    return null;
  }

  const chartData: SparklineDatum[] = trend.points.map((point) => ({
    name: title,
    x: point.label,
    y: point.value,
  }));

  const formatTooltip = (datum: SparklineDatum) =>
    intl.formatMessage(messages.trendTooltip, {
      date: datum.x,
      metric: title,
      value: datum.y,
    });

  const trendDayCount = trend.points.length.toString();

  return (
    <div ref={containerRef} className="hypershell-dashboard-sparkline-chart">
      <div className="hypershell-dashboard-sparkline-chart__plot">
        <ChartGroup
          ariaDesc={title}
          ariaTitle={title}
          containerComponent={
            <ChartVoronoiContainer
              constrainToVisibleArea
              labels={({ datum }) => formatTooltip(datum as SparklineDatum)}
            />
          }
          height={SPARKLINE_PLOT_HEIGHT}
          padding={{ bottom: 1, left: 2, right: 2, top: 1 }}
          themeColor={ChartThemeColor.blue}
          width={width}
        >
          <ChartArea data={chartData} />
        </ChartGroup>
      </div>
      <small className="hypershell-dashboard-sparkline-chart__caption">
        {intl.formatMessage(messages.trendLastDays, { days: trendDayCount })}
      </small>
    </div>
  );
}
