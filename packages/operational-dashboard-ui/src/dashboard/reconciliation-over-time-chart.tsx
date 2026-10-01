import { Card, CardBody } from "@patternfly/react-core";
import { useEffect, useRef, useState } from "react";
import { useIntl } from "react-intl";

import type { OperationalMetric } from "../application/dashboard-types";
import { messages } from "../messages";
import {
  Chart,
  ChartAxis,
  ChartGroup,
  ChartLine,
  ChartVoronoiContainer,
} from "../patternfly/victory-charts";
import "../pages/dashboard-widget.css";

const SUCCESSES_COLOR = "#3e8635";
const RETRIES_COLOR = "#f0ab00";
const FAILURES_COLOR = "#c9190b";
const CHART_HEIGHT = 200;

interface ChartDatum {
  name: string;
  x: number;
  y: number;
}

function extractHour(label: string): string {
  const tIndex = label.indexOf("T");
  return tIndex >= 0 ? label.slice(tIndex + 1) : label;
}

function buildSeriesData(
  metric: OperationalMetric | undefined,
  name: string,
): ChartDatum[] | undefined {
  if (!metric?.hourlyTrend || metric.hourlyTrend.points.length < 2) {
    return undefined;
  }
  return metric.hourlyTrend.points.map((point, i) => ({
    name,
    x: i,
    y: point.value,
  }));
}

function hasAnyTrendData(metric: OperationalMetric | undefined): boolean {
  return (metric?.hourlyTrend?.points.length ?? 0) > 0;
}

export function ReconciliationOverTimeChart({
  failures,
  retries,
  successes,
}: Readonly<{
  failures: OperationalMetric | undefined;
  retries: OperationalMetric | undefined;
  successes: OperationalMetric | undefined;
}>) {
  const intl = useIntl();
  const containerRef = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(300);

  const successesLabel = intl.formatMessage(
    messages.reconciliationChartLegendSuccesses,
  );
  const retriesLabel = intl.formatMessage(
    messages.reconciliationChartLegendRetries,
  );
  const failuresLabel = intl.formatMessage(
    messages.reconciliationChartLegendFailures,
  );

  const successesData = buildSeriesData(successes, successesLabel);
  const retriesData = buildSeriesData(retries, retriesLabel);
  const failuresData = buildSeriesData(failures, failuresLabel);

  const hasData =
    successesData !== undefined ||
    retriesData !== undefined ||
    failuresData !== undefined;

  const isAccumulating =
    !hasData &&
    (hasAnyTrendData(successes) ||
      hasAnyTrendData(retries) ||
      hasAnyTrendData(failures));

  const referencePoints =
    (successes?.hourlyTrend ?? failures?.hourlyTrend ?? retries?.hourlyTrend)
      ?.points ?? [];
  const xIndexToLabel = new Map(
    referencePoints.map((p, i) => [i, extractHour(p.label)]),
  );

  const legendData: { name: string; symbol: { fill: string } }[] = [
    ...(successesData
      ? [{ name: successesLabel, symbol: { fill: SUCCESSES_COLOR } }]
      : []),
    ...(retriesData
      ? [{ name: retriesLabel, symbol: { fill: RETRIES_COLOR } }]
      : []),
    ...(failuresData
      ? [{ name: failuresLabel, symbol: { fill: FAILURES_COLOR } }]
      : []),
  ];

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

  return (
    <Card isFullHeight isPlain>
      <CardBody>
        {!hasData ? (
          <div
            style={{
              alignItems: "center",
              display: "flex",
              height: `${String(CHART_HEIGHT)}px`,
              justifyContent: "center",
            }}
          >
            <span>
              {intl.formatMessage(
                isAccumulating
                  ? messages.reconciliationChartAccumulating
                  : messages.reconciliationChartNoData,
              )}
            </span>
          </div>
        ) : (
          <div ref={containerRef} style={{ width: "100%" }}>
            <Chart
              ariaDesc={intl.formatMessage(
                messages.widgetReconciliationOverTime,
              )}
              ariaTitle={intl.formatMessage(
                messages.widgetReconciliationOverTime,
              )}
              containerComponent={
                <ChartVoronoiContainer
                  constrainToVisibleArea
                  labels={({ datum }: { datum: ChartDatum }) =>
                    `${datum.name}: ${String(Math.round(datum.y))}`
                  }
                  voronoiDimension="x"
                />
              }
              height={CHART_HEIGHT}
              legendData={legendData}
              legendPosition="bottom"
              padding={{ bottom: 60, left: 50, right: 10, top: 10 }}
              width={width}
            >
              <ChartAxis
                tickFormat={(tick: number) =>
                  xIndexToLabel.get(Math.round(tick)) ?? ""
                }
                tickValues={[0, 6, 12, 18, 23]}
              />
              <ChartAxis
                dependentAxis
                showGrid
                tickFormat={(tick: number) => String(Math.round(tick))}
              />
              <ChartGroup>
                {successesData ? (
                  <ChartLine
                    data={successesData}
                    name={successesLabel}
                    style={{
                      data: { stroke: SUCCESSES_COLOR, strokeWidth: 2 },
                    }}
                  />
                ) : null}
                {retriesData ? (
                  <ChartLine
                    data={retriesData}
                    name={retriesLabel}
                    style={{ data: { stroke: RETRIES_COLOR, strokeWidth: 2 } }}
                  />
                ) : null}
                {failuresData ? (
                  <ChartLine
                    data={failuresData}
                    name={failuresLabel}
                    style={{ data: { stroke: FAILURES_COLOR, strokeWidth: 2 } }}
                  />
                ) : null}
              </ChartGroup>
            </Chart>
          </div>
        )}
      </CardBody>
    </Card>
  );
}
