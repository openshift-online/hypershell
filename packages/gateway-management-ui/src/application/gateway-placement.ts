export function normalizeGatewayPlacementClusterIds(
  clusterIds: readonly string[],
): string[] {
  return [
    ...new Set(clusterIds.map((clusterId) => clusterId.trim()).filter(Boolean)),
  ].sort();
}
