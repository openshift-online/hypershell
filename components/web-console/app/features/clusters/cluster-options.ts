export interface ClusterOption {
  description: string;
  id: string;
  name: string;
}

/**
 * Initial placement data while HyperShell runs in single-cluster mode.
 * This boundary can be replaced by the cluster query without changing the
 * cluster cards, provisioning links, or selected-cluster form context.
 */
export const localClusterOption: ClusterOption = {
  description: "The cluster running HyperShell.",
  id: "local",
  name: "Local cluster",
};

export const availableClusterOptions: readonly ClusterOption[] = [
  localClusterOption,
];

export function getSelectedCluster(
  clusters: readonly ClusterOption[],
  requestedId: string | null,
): ClusterOption {
  return (
    clusters.find((cluster) => cluster.id === requestedId) ??
    clusters[0] ??
    localClusterOption
  );
}

export function getGatewayProvisionPath(clusterId: string): string {
  const search = new URLSearchParams({ cluster: clusterId });
  return `/admin/gateways/new?${search.toString()}`;
}
