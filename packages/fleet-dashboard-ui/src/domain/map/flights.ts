// Promotion "fly-in" detection: a PURE diff of two map models (previous poll vs
// current) that spots a bundle landing on a downstream node and names where it flew
// FROM. When a node's deployed bundle (seed) changes to one that an UPSTREAM node was
// already running in the previous model, that is a promotion arriving - so the UI can
// launch the identicon on an arc from the source node to this one. No DOM, no layout,
// no timing here: just which flights to run, derived from server state alone.

import type { MapModel } from "./model";

export interface Flight {
  /** Node the bundle flew FROM (an upstream node in the previous model). Empty for a
   *  tray flight, whose source is the release tray / change head, not a node. */
  readonly sourceId: string;
  /** Node the bundle landed ON (changed seed in the current model). */
  readonly destId: string;
  /** The landed bundle's digest seed (identifies the identicon to fly). */
  readonly seed: string;
  /** The dest node's seed BEFORE this flight - what its card should keep showing until
   *  the flying copy lands (so the dest doesn't switch icons before the bundle arrives). */
  readonly destPrevSeed: string;
  /** True when the bundle entered at the first column from the release tray (no upstream
   *  node held it): a brand-new frontier bundle completing the chain tray -> int. */
  readonly fromTray?: boolean;
}

/** Column order index by key, from the model's ordered columns (left -> right). */
function columnIndexByKey(model: MapModel): Map<string, number> {
  const index = new Map<string, number>();
  model.columns.forEach((c, i) => index.set(c.key, i));
  return index;
}

/**
 * Flights to animate for the transition prev -> next. A flight is emitted when a node
 * in `next` has a non-empty seed that (a) differs from that same node's seed in `prev`
 * and (b) was the ACTIVE seed of some node sitting in an earlier column in `prev`. The
 * source is the NEAREST such upstream node (largest column index still left of the
 * dest), which matches a one-stage promotion hop. Returns [] when there is no prior
 * model (first render) so the map never "flies" on initial load.
 */
export function computeFlights(
  prev: MapModel | null,
  next: MapModel,
): Flight[] {
  if (!prev) return [];

  const prevSeedById = new Map(prev.nodes.map((n) => [n.id, n.seed]));
  const nextCol = columnIndexByKey(next);
  const prevCol = columnIndexByKey(prev);

  const flights: Flight[] = [];
  for (const node of next.nodes) {
    if (node.seed === "") continue;
    const before = prevSeedById.get(node.id);
    // Only a genuine change lands a flight (unchanged polls and brand-new nodes don't).
    if (before === undefined || before === node.seed) continue;
    const destIdx = nextCol.get(node.columnKey);
    if (destIdx === undefined) continue;

    let source: { id: string; idx: number } | null = null;
    for (const p of prev.nodes) {
      if (p.id === node.id || p.seed !== node.seed) continue;
      const srcIdx = prevCol.get(p.columnKey);
      if (srcIdx === undefined || srcIdx >= destIdx) continue;
      // Nearest upstream wins (the stage the bundle most plausibly promoted from).
      if (!source || srcIdx > source.idx) source = { id: p.id, idx: srcIdx };
    }
    if (source) {
      flights.push({
        sourceId: source.id,
        destId: node.id,
        seed: node.seed,
        destPrevSeed: before,
      });
    } else if (destIdx === 0) {
      // First column, no upstream node held this bundle: a brand-new frontier arriving
      // from the release tray. Fly it in from the tray so the chain reads tray -> int
      // -> stage -> prod rather than int just snapping to the new bundle.
      flights.push({
        sourceId: "",
        destId: node.id,
        seed: node.seed,
        destPrevSeed: before,
        fromTray: true,
      });
    }
  }
  return flights;
}
