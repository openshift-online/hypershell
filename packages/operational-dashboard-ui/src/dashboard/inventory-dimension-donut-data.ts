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

const unknownBucketKey = "unknown";

export function inventoryDimensionLabel(
  intl: IntlShape,
  dimensionKey: string,
): string {
  if (dimensionKey === unknownBucketKey) {
    return intl.formatMessage(messages.inventoryStatusUnknown);
  }

  return dimensionKey;
}

function inventoryDimensionColor(index: number): string {
  return (
    INVENTORY_STATUS_COLORS[index % INVENTORY_STATUS_COLORS.length] ?? "#0066cc"
  );
}

export function sortInventoryDimensionEntries(
  buckets: Record<string, number>,
): { count: number; label: string }[] {
  return Object.entries(buckets)
    .filter(([, count]) => count > 0)
    .map(([label, count]) => ({ count, label }))
    .sort((left, right) => {
      if (right.count !== left.count) {
        return right.count - left.count;
      }

      return left.label.localeCompare(right.label);
    });
}

export function buildInventoryDimensionDonutData(
  intl: IntlShape,
  buckets: Record<string, number> | undefined,
): StatusDonutSeries {
  if (buckets === undefined) {
    return { colorScale: [], data: [], legendData: [] };
  }

  const sortedEntries = sortInventoryDimensionEntries(buckets);

  return buildStatusDonutData(
    sortedEntries.map(({ label: dimensionKey, count }, index) => {
      const label = inventoryDimensionLabel(intl, dimensionKey);
      const color =
        dimensionKey === unknownBucketKey
          ? INVENTORY_STATUS_UNKNOWN_COLOR
          : inventoryDimensionColor(index);

      return {
        color,
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
