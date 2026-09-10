import { Button, Tooltip } from "@patternfly/react-core";
import { TrendDownIcon, TrendUpIcon } from "@patternfly/react-icons";
import type { MessageDescriptor } from "react-intl";
import { useIntl } from "react-intl";

import { messages } from "../messages";
import type { MetricTrendChange } from "./metric-trend-change";
import "../pages/dashboard-widget.css";

export interface MetricTrendTooltipMessages {
  decrease: MessageDescriptor;
  increase: MessageDescriptor;
}

export function MetricTrendIndicator({
  trendChange,
  tooltipMessages,
}: Readonly<{
  trendChange: MetricTrendChange;
  tooltipMessages?: MetricTrendTooltipMessages;
}>) {
  const intl = useIntl();
  const isIncrease = trendChange.direction === "increase";
  const tooltipMessage = isIncrease
    ? (tooltipMessages?.increase ?? messages.summaryTrendIncrease)
    : (tooltipMessages?.decrease ?? messages.summaryTrendDecrease);
  const tooltipContent = intl.formatMessage(tooltipMessage, {
    percent: trendChange.percent,
  });

  return (
    <Tooltip content={tooltipContent} aria="labelledby">
      <Button
        aria-label={tooltipContent}
        className={
          isIncrease
            ? "hypershell-dashboard-summary-trend hypershell-dashboard-summary-trend--increase"
            : "hypershell-dashboard-summary-trend hypershell-dashboard-summary-trend--decrease"
        }
        isInline
        variant="plain"
      >
        {isIncrease ? <TrendUpIcon /> : <TrendDownIcon />}
      </Button>
    </Tooltip>
  );
}
