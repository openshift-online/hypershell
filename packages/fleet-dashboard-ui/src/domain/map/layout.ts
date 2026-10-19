// Map LAYOUT: pure geometry over the MapModel. Given columns (promotion order),
// the per-node HUB/SPOKE role and the node set, it computes absolute SVG
// coordinates. No DOM, no measurement - deterministic from the model alone so it
// is unit-testable and the presentation layer just renders the boxes it returns.
//
// GEOMETRY CONTRACT (specs/platform/fleet-dashboard-map-hub-spine-layout.spec.md):
// every column's governing HUB sits on ONE shared horizontal line - the hub spine -
// at the vertical center of the map, and the change diamond + every gate ride that
// same line. SPOKEs stack symmetrically above and below the spine. The HUB/SPOKE
// role, not the cloud provider, governs vertical placement; nothing here consults a
// provider lane, a baked-in hub list or a provider allowlist.

import type { MapColumn, MapModel, MapNode } from "./model";

/** Fixed card + spacing constants (SVG user units). Tuned to the prototype. */
export const NODE_W = 150;
// Tall enough for TWO bottom "chins": the gateway-history sand sparkline and, below
// it, the per-cluster sandbox strip (see map-node). Raised from 106 when the second
// chin was added.
export const NODE_H = 124;
export const COL_GAP = 190;
export const V_GAP = 46;
export const MARGIN_X = 210;
export const MARGIN_TOP = 96;
export const MARGIN_BOTTOM = 48;

export interface NodeBox {
  readonly id: string;
  readonly x: number;
  readonly y: number;
  readonly w: number;
  readonly h: number;
  readonly cx: number;
  readonly cy: number;
}

export interface ColumnLayout {
  readonly key: string;
  readonly index: number;
  readonly x: number;
  readonly centerX: number;
}

export interface GateLayout {
  readonly id: string;
  readonly x: number;
  readonly y: number;
  readonly fromColumnKey: string;
  readonly toColumnKey: string;
}

/** An env-type header band spanning the consecutive columns that share a label. */
export interface BandLayout {
  readonly label: string;
  readonly centerX: number;
  readonly x: number;
  readonly width: number;
}

export interface MapLayout {
  readonly nodes: readonly NodeBox[];
  readonly columns: readonly ColumnLayout[];
  readonly bands: readonly BandLayout[];
  readonly gates: readonly GateLayout[];
  /**
   * Y of the promotion spine - the single horizontal line every column's HUB sits
   * on, which the change diamond and every gate ride. Equals the vertical center of
   * the laid-out map (never the top of a lane), with SPOKEs stacked symmetrically
   * above and below it.
   */
  readonly hubSpineY: number;
  readonly width: number;
  readonly height: number;
  readonly nodeW: number;
  readonly nodeH: number;
}

/** Row pitch: one card height plus the vertical gap between stacked rows. */
const ROW_PITCH = NODE_H + V_GAP;

/**
 * Compute absolute positions for every node, column and gate. Columns are evenly
 * spaced left -> right in model order. Within a column the governing HUB takes the
 * spine (row 0) and its SPOKEs stack symmetrically around it; the spine itself sits
 * at the vertical center of the map so the change diamond and every gate run
 * straight through the HUB cards. Placement is driven by the HUB/SPOKE role alone -
 * no provider lane, hub table or provider list is consulted.
 */
export function computeLayout(model: MapModel): MapLayout {
  const columns: ColumnLayout[] = model.columns.map((c: MapColumn) => {
    const x = MARGIN_X + c.index * (NODE_W + COL_GAP);
    return { key: c.key, index: c.index, x, centerX: x + NODE_W / 2 };
  });
  const colByKey = new Map(columns.map((c) => [c.key, c]));

  // Env-type bands: merge runs of adjacent columns sharing an envLabel into one
  // header span (so int/stage/prod read as grouped bands above their columns).
  const bands: BandLayout[] = [];
  model.columns.forEach((mc, i) => {
    const col = columns[i];
    if (!col) {
      return;
    }
    const label = mc.envLabel;
    if (label === null || label === "") {
      return;
    }
    const left = col.x;
    const right = col.x + NODE_W;
    const prev = bands[bands.length - 1];
    if (prev?.label === label) {
      const newRight = right;
      const newX = Math.min(prev.x, left);
      bands[bands.length - 1] = {
        label,
        x: newX,
        width: newRight - newX,
        centerX: (newX + newRight) / 2,
      };
    } else {
      bands.push({
        label,
        x: left,
        width: right - left,
        centerX: (left + right) / 2,
      });
    }
  });

  const byId = new Map(model.nodes.map((n) => [n.id, n]));

  // Assign every node a SIGNED row offset around its column's spine: 0 is the spine
  // (the governing HUB, or the first node when a column has no hub), negative is
  // above, positive is below. SPOKEs fill rings 1, 2, ... alternating below then
  // above so they stack symmetrically; the order is the column's node order, so the
  // same model always yields the same assignment. `halfRows` is the deepest ring any
  // column needs on either side - the spine reserves that many rows above AND below
  // itself, which centers it exactly whichever side the spokes fall on.
  interface Placement {
    readonly id: string;
    readonly columnKey: string;
    readonly rowOffset: number;
  }
  const placements: Placement[] = [];
  let halfRows = 0;
  for (const column of model.columns) {
    const colNodes = column.nodeIds
      .map((id) => byId.get(id))
      .filter((n): n is MapNode => n !== undefined);
    if (colNodes.length === 0) {
      continue;
    }
    // HUB-first: the governing hub takes the spine no matter where it sits in the
    // input; without a hub the first node holds the spine (the no-hub fallback).
    const hubIdx = colNodes.findIndex((n) => n.isHub);
    const spineIdx = hubIdx === -1 ? 0 : hubIdx;
    const spine = colNodes[spineIdx];
    if (spine) {
      placements.push({ id: spine.id, columnKey: column.key, rowOffset: 0 });
    }
    const spokes = colNodes.filter((_, i) => i !== spineIdx);
    spokes.forEach((spoke, i) => {
      const ring = Math.floor(i / 2) + 1;
      const rowOffset = i % 2 === 0 ? ring : -ring;
      placements.push({ id: spoke.id, columnKey: column.key, rowOffset });
      if (ring > halfRows) {
        halfRows = ring;
      }
    });
  }

  // The hub spine is the vertical center of the laid-out CONTENT: the content region
  // reserves `halfRows` rows above and below the spine, so the spine Y is exactly its
  // midpoint and no provider, count or ordering can push it off center. (This is the
  // content midpoint, not the full SVG box midpoint - MARGIN_TOP and MARGIN_BOTTOM
  // differ on purpose, so do not "recenter" against `height`.)
  const hubSpineY = MARGIN_TOP + halfRows * ROW_PITCH + NODE_H / 2;
  const nodes: NodeBox[] = [];
  for (const p of placements) {
    const col = colByKey.get(p.columnKey);
    if (!col) {
      continue;
    }
    const x = col.x;
    const y = hubSpineY + p.rowOffset * ROW_PITCH - NODE_H / 2;
    nodes.push({
      id: p.id,
      x,
      y,
      w: NODE_W,
      h: NODE_H,
      cx: x + NODE_W / 2,
      cy: y + NODE_H / 2,
    });
  }

  // The change diamond and every gate ride the hub spine, so the whole promotion
  // line runs straight through the HUB cards from left to right.
  const gates: GateLayout[] = [];
  for (const g of model.gates) {
    const from = colByKey.get(g.fromColumnKey);
    if (!from) {
      continue;
    }
    const to = colByKey.get(g.toColumnKey);
    // Normal gate: midway between its source and the next column. Terminal gate (no
    // downstream column): a half-column past the right edge of the final stage.
    const x = to ? (from.x + NODE_W + to.x) / 2 : from.x + NODE_W + COL_GAP / 2;
    gates.push({
      id: g.id,
      x,
      y: hubSpineY,
      fromColumnKey: g.fromColumnKey,
      toColumnKey: g.toColumnKey,
    });
  }

  const lastCol = columns[columns.length - 1];
  const width = lastCol ? lastCol.x + NODE_W + MARGIN_X : MARGIN_X * 2 + NODE_W;
  // Content is symmetric about the spine (`halfRows` rows each side), so the bottom
  // mirrors the top margin's reserved rows.
  const contentBottom = hubSpineY + halfRows * ROW_PITCH + NODE_H / 2;
  const height = contentBottom + MARGIN_BOTTOM;

  return {
    nodes,
    columns,
    bands,
    gates,
    hubSpineY,
    width,
    height,
    nodeW: NODE_W,
    nodeH: NODE_H,
  };
}
