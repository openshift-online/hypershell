// Release "freight" bundles for the map's bottom bar. Pure projection over the
// promotion plane's release set + the map nodes - it answers "what release
// bundles exist, newest first, and which instances run each one". No fleet
// identity here: digests/versions are opaque server data and the instance list
// comes from the already-firewall-clean nodes.

import type { ReleaseBundle } from "../promotion";
import type { MapNode } from "./model";

/**
 * Release bundles ordered newest -> oldest: by date when the server supplies one
 * (ISO strings compare lexically), then by version descending, then by digest for
 * a fully-stable tie-break. Bundles without a date sort after dated ones.
 */
export function bundleList(
  releaseByDigest: Readonly<Record<string, ReleaseBundle>>,
): readonly ReleaseBundle[] {
  return Object.values(releaseByDigest)
    .slice()
    .sort((a, b) => {
      if (a.date && b.date && a.date !== b.date) {
        return a.date < b.date ? 1 : -1;
      }
      if (a.date !== b.date) {
        return a.date ? -1 : 1; // dated before undated
      }
      if (a.version !== b.version) {
        return a.version < b.version ? 1 : -1;
      }
      return (a.digest ?? "") < (b.digest ?? "") ? 1 : -1;
    });
}

/** Stable identicon/identiname seed for a bundle: its digest, else its version. */
export function seedForBundle(bundle: ReleaseBundle): string {
  return bundle.digest ?? bundle.version;
}

/**
 * Nodes currently running a given bundle, matched on digest when both sides have
 * one (the precise key), else on version. Returns them in node order.
 */
export function deployedFor(
  bundle: ReleaseBundle,
  nodes: readonly MapNode[],
): readonly MapNode[] {
  return nodes.filter((n) => {
    if (bundle.digest && n.digest) {
      return n.digest === bundle.digest;
    }
    return n.version !== null && n.version === bundle.version;
  });
}

/** Count of nodes running a bundle - the badge on each freight card. */
export function deployedCount(
  bundle: ReleaseBundle,
  nodes: readonly MapNode[],
): number {
  return deployedFor(bundle, nodes).length;
}
