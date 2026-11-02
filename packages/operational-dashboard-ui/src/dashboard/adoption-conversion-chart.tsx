import { useMemo } from "react";
import { useIntl } from "react-intl";

import type { OperationalMetric } from "../application/dashboard-types";
import { messages } from "../messages";
import { buildStatusDonutData } from "./status-donut-data";
import { STATUS_DONUT_COLORS } from "./status-donut-colors";
import { StatusDonutChart } from "./status-donut-chart";
import type { StatusDonutDatum } from "./status-donut-data";

function parseAdoptionConversionCounts(
  metric: OperationalMetric,
): { firstGatewayUsers: number; notConvertedUsers: number } | undefined {
  const registeredUsers = Number(metric.value);
  const firstGatewayUsers = Number(metric.usersWithGateway);

  if (
    !Number.isFinite(registeredUsers) ||
    !Number.isFinite(firstGatewayUsers) ||
    registeredUsers <= 0 ||
    firstGatewayUsers < 0 ||
    firstGatewayUsers > registeredUsers
  ) {
    return undefined;
  }

  return {
    firstGatewayUsers,
    notConvertedUsers: registeredUsers - firstGatewayUsers,
  };
}

export function AdoptionConversionChart({
  metric,
}: Readonly<{ metric: OperationalMetric }>) {
  const intl = useIntl();
  const counts = parseAdoptionConversionCounts(metric);
  const donutData = useMemo(() => {
    if (counts === undefined || metric.gatewayConversionPercent === undefined) {
      return undefined;
    }

    const firstGatewayLabel = intl.formatMessage(
      messages.adoptionFirstGatewayConversion,
    );
    const notConvertedLabel = intl.formatMessage(messages.adoptionNotConverted);

    return buildStatusDonutData([
      {
        color: STATUS_DONUT_COLORS.provisioning,
        count: counts.firstGatewayUsers,
        label: firstGatewayLabel,
        legendName: intl.formatMessage(messages.statusDonutLegend, {
          count: counts.firstGatewayUsers,
          status: firstGatewayLabel,
        }),
      },
      {
        color: STATUS_DONUT_COLORS.unused,
        count: counts.notConvertedUsers,
        label: notConvertedLabel,
        legendName: intl.formatMessage(messages.statusDonutLegend, {
          count: counts.notConvertedUsers,
          status: notConvertedLabel,
        }),
      },
    ]);
  }, [counts, intl, metric.gatewayConversionPercent]);

  if (donutData === undefined) {
    return null;
  }

  return (
    <StatusDonutChart
      ariaDesc={intl.formatMessage(messages.adoptionConversionAriaDesc)}
      ariaTitle={intl.formatMessage(messages.adoptionConversionChartTitle)}
      colorScale={donutData.colorScale}
      data={donutData.data}
      dataLabel={(datum: StatusDonutDatum) =>
        datum.x
          ? intl.formatMessage(messages.statusDonutDataLabel, {
              count: datum.y,
              status: datum.x,
            })
          : null
      }
      legendData={donutData.legendData}
      size="compact"
      subTitle={intl.formatMessage(messages.adoptionConversionSubtitle)}
      title={intl.formatMessage(messages.adoptionConversionRate, {
        rate: metric.gatewayConversionPercent,
      })}
    />
  );
}
