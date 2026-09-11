import {
  Content,
  DescriptionList,
  DescriptionListDescription,
  DescriptionListGroup,
  DescriptionListTerm,
  Stack,
  StackItem,
} from "@patternfly/react-core";
import { useMemo } from "react";
import { FormattedMessage, useIntl } from "react-intl";

import type { OperationalMetric } from "../application/dashboard-types";
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
    <Stack hasGutter className="hypershell-dashboard-provision-time">
      <StackItem>
        <DescriptionList
          isHorizontal
          aria-label={intl.formatMessage(messages.provisionTimeStatsAriaLabel)}
        >
          {STAT_ROWS.map((row) => (
            <DescriptionListGroup key={row.key} className="pf-v6-u-pb-sm">
              <DescriptionListTerm>
                <FormattedMessage {...row.label} />
              </DescriptionListTerm>
              <DescriptionListDescription>
                {formatProvisionDurationValue(
                  intl,
                  statValues[row.key],
                  metric.unit,
                )}
              </DescriptionListDescription>
            </DescriptionListGroup>
          ))}
        </DescriptionList>
      </StackItem>
      <StackItem>
        <Content
          component="p"
          className="hypershell-dashboard-provision-time__note"
        >
          <FormattedMessage
            {...messages.provisionTimeP95Note}
            values={{
              duration: stats.p95Minutes.toFixed(2),
              unit: metric.unit ?? "",
            }}
          />
        </Content>
      </StackItem>
    </Stack>
  );
}
