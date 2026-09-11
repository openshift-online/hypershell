import type { IntlShape } from "react-intl";

import { messages } from "../messages";
import {
  INVENTORY_STATUS_COLORS,
  INVENTORY_STATUS_UNKNOWN_COLOR,
} from "./inventory-status-colors";
import {
  buildStatusDonutData,
  type StatusDonutSeries,
} from "./status-donut-data";

const maxInventoryStatusBuckets = 5;
const unknownBucketKey = "unknown";

function inventoryStatusLabel(intl: IntlShape, statusKey: string): string {
  if (statusKey === unknownBucketKey) {
    return intl.formatMessage(messages.inventoryStatusUnknown);
  }

  return statusKey;
}

function inventoryStatusColor(statusKey: string, index: number): string {
  if (statusKey === unknownBucketKey) {
    return INVENTORY_STATUS_UNKNOWN_COLOR;
  }

  return (
    INVENTORY_STATUS_COLORS[index % INVENTORY_STATUS_COLORS.length] ?? "#0066cc"
  );
}

export function buildInventoryStatusData(
  intl: IntlShape,
  inventoryStatus: Record<string, number> | undefined,
): StatusDonutSeries {
  if (inventoryStatus === undefined) {
    return { colorScale: [], data: [], legendData: [] };
  }

  const nonZeroEntries = Object.entries(inventoryStatus).filter(
    ([, count]) => count > 0,
  );

  if (nonZeroEntries.length > maxInventoryStatusBuckets) {
    return { colorScale: [], data: [], legendData: [] };
  }

  const sortedEntries = nonZeroEntries.sort(([left], [right]) =>
    left.localeCompare(right),
  );

  return buildStatusDonutData(
    sortedEntries.map(([statusKey, count], index) => {
      const label = inventoryStatusLabel(intl, statusKey);

      return {
        color: inventoryStatusColor(statusKey, index),
        count,
        label,
        legendName: intl.formatMessage(messages.statusDonutLegend, {
          count,
          status: label,
        }),
      };
    }),
  );
}
