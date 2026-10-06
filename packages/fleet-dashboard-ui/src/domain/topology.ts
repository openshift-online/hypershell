// Topology plane: each hub instance's decoded hub/spoke layout, as the BFF reads
// it from the per-instance topology ConfigMaps (/api/topology). The dashboard uses
// it to attribute a hub's gateways/sandboxes to the managed cluster (spoke) they
// run on - nesting co-located spokes under the hub and linking out to remote ones.
//
// FIREWALL: nothing about the fleet is baked in here. Every name (hub instance,
// dns label, spoke names) arrives at runtime inside the payload; the hub <-> spoke
// relationships are data, discovered per request, never a compiled-in table.

/** A hub's own identity + the spokes it manages from a different cluster. */
export interface TopologyHub {
  /** The hub instance name (e.g. the delivery label value), opaque data. */
  readonly instance: string;
  /** The hub's DNS label, when the server reports one. */
  readonly dnsLabel: string | null;
  /** Managed clusters this hub drives that live on a DIFFERENT cluster. */
  readonly remoteSpokes: readonly string[];
}

/** A managed cluster (spoke) co-located on the hub's own cluster. */
export interface TopologySpoke {
  /** The spoke's managed-cluster name (== GitOps spoke name), opaque data. */
  readonly name: string;
}

/** One instance's decoded topology document. */
export interface InstanceTopology {
  /** The instance this topology describes (the record key, carried for clarity). */
  readonly instance: string;
  /** The hub block, or null when the document omits one (spoke-only / malformed). */
  readonly hub: TopologyHub | null;
  /** Co-located managed clusters, as the document lists them. */
  readonly spokes: readonly TopologySpoke[];
}

/** The /api/topology plane: a map keyed by instance name. */
export type TopologyData = Readonly<Record<string, InstanceTopology>>;
