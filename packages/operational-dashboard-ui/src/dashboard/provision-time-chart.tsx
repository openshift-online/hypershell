import { Content } from "@patternfly/react-core";
import { useMemo } from "react";
import { FormattedMessage, useIntl } from "react-intl";

import type { OperationalMetric } from "../application/dashboard-types";
import { DashboardStatPanel } from "./dashboard-stat-panel";
import { messages } from "../messages";
import {
  formatProvisionDurationValue,
  parseProvisionDurationStats,
} from "./provision-time-data";

const STAT_ROWS = [
  { key: "mean", label: messages.provisionTimeAverage },
  { key: "p50", label: messages.provisionTimeMedian },
  { key: "p95", label: messages.provisionTimeP95Label },
] as const;

export function ProvisionTimeChart({
  metric,
}: Readonly<{ metric: OperationalMetric }>) {
  const intl = useIntl();
  const stats = useMemo(() => parseProvisionDurationStats(metric), [metric]);

  if (!stats) {
    return null;
  }

  const statValues = {
    mean: stats.meanMinutes,
    p50: stats.p50Minutes,
    p95: stats.p95Minutes,
  } as const;

  return (
    <DashboardStatPanel
      ariaLabel={intl.formatMessage(messages.provisionTimeStatsAriaLabel)}
      footer={
        <Content
          component="p"
          className="hypershell-dashboard-stat-panel__footer"
        >
          <FormattedMessage
            {...messages.provisionTimeP95Note}
            values={{
              duration: stats.p95Minutes.toFixed(2),
              unit: metric.unit ?? "",
            }}
          />
        </Content>
      }
      rows={STAT_ROWS.map((row) => ({
        id: row.key,
        label: <FormattedMessage {...row.label} />,
        value: formatProvisionDurationValue(
          intl,
          statValues[row.key],
          metric.unit,
        ),
      }))}
    />
  );
}
