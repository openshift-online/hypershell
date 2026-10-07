import {
  Button,
  Flex,
  FormSelect,
  FormSelectOption,
  Label,
} from "@patternfly/react-core";
import { AddWidgetsButton } from "@patternfly/widgetized-dashboard";
import { FormattedMessage, useIntl } from "react-intl";

import type { DashboardTimeRange } from "../application/dashboard-types";
import { messages } from "../messages";
import { ResourceRefreshButton } from "../shared/resource-refresh-button";
import {
  DashboardSecondaryNav,
  type DashboardSecondaryNavActive,
} from "./dashboard-secondary-nav";

export function DashboardNavigation({
  active,
  isRefreshing,
  onRefresh,
  updatedMinutesAgo,
}: Readonly<{
  active: DashboardSecondaryNavActive;
  isRefreshing: boolean;
  onRefresh: () => void;
  updatedMinutesAgo?: number;
}>) {
  const intl = useIntl();

  return (
    <div className="hypershell-dashboard-navigation">
      <DashboardSecondaryNav active={active} />
      <Flex alignItems={{ default: "alignItemsCenter" }}>
        <ResourceRefreshButton
          ariaLabel={intl.formatMessage(messages.refresh)}
          isRefreshing={isRefreshing}
          onRefresh={onRefresh}
        />
        {updatedMinutesAgo !== undefined ? (
          <Label color="blue">
            {updatedMinutesAgo < 1 ? (
              <FormattedMessage {...messages.updatedUnderMinuteAgo} />
            ) : updatedMinutesAgo === 1 ? (
              <FormattedMessage {...messages.updatedMinuteAgo} />
            ) : (
              <FormattedMessage
                {...messages.updatedMinutesAgo}
                values={{ minutes: updatedMinutesAgo }}
              />
            )}
          </Label>
        ) : null}
      </Flex>
    </div>
  );
}

export function DashboardFilterToolbar({
  onAddWidgets,
  onResetToDefault,
  onTimeRangeChange,
  showTimeRange = true,
  timeRange,
}: Readonly<{
  onAddWidgets: () => void;
  onResetToDefault: () => void;
  onTimeRangeChange?: (timeRange: DashboardTimeRange) => void;
  showTimeRange?: boolean;
  timeRange?: DashboardTimeRange;
}>) {
  const intl = useIntl();

  return (
    <Flex
      alignItems={{ default: "alignItemsCenter" }}
      justifyContent={{ default: "justifyContentSpaceBetween" }}
    >
      <Flex alignItems={{ default: "alignItemsCenter" }}>
        {showTimeRange && timeRange !== undefined && onTimeRangeChange ? (
          <FormSelect
            aria-label={intl.formatMessage(messages.timeRange)}
            style={{ minWidth: 130 }}
            value={String(timeRange)}
            onChange={(_event, value) => {
              onTimeRangeChange(Number(value) as DashboardTimeRange);
            }}
          >
            <FormSelectOption
              value="7"
              label={intl.formatMessage(messages.timeRangeLast7Days)}
            />
            <FormSelectOption
              value="30"
              label={intl.formatMessage(messages.timeRangeLast30Days)}
            />
          </FormSelect>
        ) : null}
      </Flex>
      <Flex alignItems={{ default: "alignItemsCenter" }}>
        <Button variant="link" onClick={onResetToDefault}>
          {intl.formatMessage(messages.resetToDefault)}
        </Button>
        <AddWidgetsButton onClick={onAddWidgets}>
          {intl.formatMessage(messages.addWidgets)}
        </AddWidgetsButton>
      </Flex>
    </Flex>
  );
}
