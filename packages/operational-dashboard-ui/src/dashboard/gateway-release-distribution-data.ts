import { sortInventoryDimensionEntries } from "./inventory-dimension-donut-data";

export function buildGatewayReleaseListEntries(
  distribution: Record<string, number> | undefined,
): { count: number; label: string }[] {
  if (distribution === undefined) {
    return [];
  }

  return sortInventoryDimensionEntries(distribution);
}
