// Map LAYOUT: pure geometry over the MapModel. Given columns (promotion order),
// the per-node HUB/SPOKE role, the per-node provider lane and the node set, it
// computes absolute SVG coordinates. No DOM, no measurement - deterministic from the
// model alone so it is unit-testable and the presentation layer just renders the
// boxes it returns.
//
// GEOMETRY CONTRACT (specs/platform/fleet-dashboard-map-hub-spine-layout.spec.md):
// two groupings hold at once under the single-cloud-hub assumption. First, every
// column's governing HUB sits on ONE shared horizontal line - the hub spine - at the
// vertical center of the map, and the change diamond + every gate ride that same line.
// Second, clusters are grouped into visible PROVIDER LANES (one per cloud provider):
// the hub-hosting lane is centered on the spine, its SPOKEs ring symmetrically around
// it, and the remaining provider lanes stack as bands above and below it, balanced.
// Every rule below is derived from the role and provider signals already on each node
// and lane; nothing here consults a baked-in hub list, a provider allowlist or a
// hard-coded provider name (including the hub-hosting provider, today IBM).

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

/**
 * A rendered provider lane: the contiguous horizontal band the lane's clusters occupy,
 * so the presentation can draw the grouping rectangle + provider label. The
 * hub-hosting lane's band straddles the centered spine; every other lane's band stacks
 * above or below it.
 */
export interface LaneLayout {
  readonly key: string;
  readonly hostsHub: boolean;
  readonly provider: string | null;
  readonly y: number;
  readonly height: number;
  readonly centerY: number;
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
  /** Provider lanes as visible grouping bands, in deterministic model-lane order. */
  readonly lanes: readonly LaneLayout[];
  readonly gates: readonly GateLayout[];
  /**
   * Y of the promotion spine - the single horizontal line every column's HUB sits
   * on, which the change diamond and every gate ride. Equals the vertical center of
   * the laid-out map (never the top of a lane): the hub-hosting provider lane is
   * centered on it, with the other provider lanes balanced above and below.
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
 * Compute absolute positions for every node, column, provider lane and gate. Columns
 * are evenly spaced left -> right in model order. Vertical placement groups clusters
 * into provider lanes around a centered hub spine: the hub-hosting lane is centered so
 * every column's governing HUB rides the spine (row 0), its SPOKEs ring symmetrically
 * around it, and the remaining provider lanes stack as balanced bands above and below.
 * The spine sits at the vertical center of the map so the change diamond and every
 * gate run straight through the HUB cards. Placement is derived from the HUB/SPOKE
 * role and the provider lane alone - no hub table, provider allowlist or provider name
 * is consulted.
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

  // Nodes of a column belonging to one provider lane, in the column's node order.
  const laneNodesInColumn = (
    column: MapColumn,
    laneKey: string,
    excludeHubs = false,
  ): MapNode[] =>
    column.nodeIds
      .map((id) => byId.get(id))
      .filter((n): n is MapNode => n !== undefined)
      .filter((n) => n.laneKey === laneKey && !(excludeHubs && n.isHub));

  // The lane that hosts the HUBs is the spine lane; it is centered so its hubs ride the
  // spine. Without any hub (degenerate fallback) the first lane in model order acts as
  // the spine lane so the diagram still has a center line. Derived from the per-lane
  // `hostsHub` signal, never a provider name.
  const hubLaneKey =
    model.lanes.find((l) => l.hostsHub)?.key ?? model.lanes[0]?.key ?? null;

  // Signed row offset from the spine for every node: 0 is the spine, negative is above,
  // positive below. Multiply by ROW_PITCH off the spine Y to get a card's center.
  const rowOffset = new Map<string, number>();
  // Rows occupied strictly above / below the spine, so later lanes stack outside the
  // earlier ones and the final centering reserves a symmetric half each side.
  let topRows = 0;
  let botRows = 0;
  // Inclusive offset range [lo, hi] each lane occupies, for its band rectangle.
  const laneRange = new Map<string, { lo: number; hi: number }>();

  // 1) Spine (hub-hosting) lane: the governing HUB of each column takes the spine
  //    (HUB-first, regardless of input order); without a hub in a column its first
  //    spine-lane node holds the spine. SPOKEs in this lane ring symmetrically around
  //    the spine on alternating rings (below then above), deterministic in node order.
  if (hubLaneKey !== null) {
    let up = 0;
    let down = 0;
    for (const column of model.columns) {
      const laneNodes = laneNodesInColumn(column, hubLaneKey);
      if (laneNodes.length === 0) {
        continue;
      }
      const hubIdx = laneNodes.findIndex((n) => n.isHub);
      const pivotIdx = hubIdx === -1 ? 0 : hubIdx;
      const pivot = laneNodes[pivotIdx];
      if (pivot) {
        rowOffset.set(pivot.id, 0);
      }
      const spokes = laneNodes.filter((_, i) => i !== pivotIdx);
      spokes.forEach((spoke, i) => {
        const ring = Math.floor(i / 2) + 1;
        const offset = i % 2 === 0 ? ring : -ring;
        rowOffset.set(spoke.id, offset);
        if (offset > down) {
          down = offset;
        }
        if (-offset > up) {
          up = -offset;
        }
      });
    }
    topRows = up;
    botRows = down;
    laneRange.set(hubLaneKey, { lo: -up, hi: down });
  }

  // Every HUB rides the spine, full stop (spec: "every HUB node's vertical center SHALL
  // equal the single hub-spine Y"). Under the single-cloud-hub assumption all hubs are
  // already in the spine lane; this also pins a hub on a different provider to the spine
  // in the out-of-scope multi-cloud-hub case, where role still wins for the HUB row.
  for (const n of model.nodes) {
    if (n.isHub) {
      rowOffset.set(n.id, 0);
    }
  }

  // 2) Remaining provider lanes: contiguous bands stacked on the lighter side so they
  //    balance above and below the spine lane. Within a lane a column's clusters stack
  //    downward from the band's top row, in model node order. HUB nodes never ride a
  //    band - role pins them to the spine above.
  for (const lane of model.lanes) {
    if (lane.key === hubLaneKey) {
      continue;
    }
    let stack = 0;
    for (const column of model.columns) {
      const count = laneNodesInColumn(column, lane.key, true).length;
      if (count > stack) {
        stack = count;
      }
    }
    if (stack === 0) {
      // A lane whose only nodes are hubs (multi-cloud-hub case): its hubs already ride
      // the spine, so there is no band to lay out here.
      continue;
    }
    let lo: number;
    let hi: number;
    if (topRows <= botRows) {
      hi = -(topRows + 1);
      lo = -(topRows + stack);
      topRows += stack;
    } else {
      lo = botRows + 1;
      hi = botRows + stack;
      botRows += stack;
    }
    laneRange.set(lane.key, { lo, hi });
    for (const column of model.columns) {
      laneNodesInColumn(column, lane.key, true).forEach((n, i) => {
        rowOffset.set(n.id, lo + i);
      });
    }
  }

  // The spine is the vertical center of the laid-out CONTENT: reserve the deepest row
  // count on EITHER side both above and below the spine, so the spine Y is exactly the
  // midpoint and no provider, count or ordering can push it off center. (This is the
  // content midpoint, not the full SVG box midpoint - MARGIN_TOP and MARGIN_BOTTOM
  // differ on purpose, so do not "recenter" against `height`.)
  const half = Math.max(topRows, botRows);
  const hubSpineY = MARGIN_TOP + half * ROW_PITCH + NODE_H / 2;

  const nodes: NodeBox[] = [];
  for (const n of model.nodes) {
    const col = colByKey.get(n.columnKey);
    const offset = rowOffset.get(n.id);
    if (!col || offset === undefined) {
      continue;
    }
    const x = col.x;
    const y = hubSpineY + offset * ROW_PITCH - NODE_H / 2;
    nodes.push({
      id: n.id,
      x,
      y,
      w: NODE_W,
      h: NODE_H,
      cx: x + NODE_W / 2,
      cy: y + NODE_H / 2,
    });
  }

  // Lane bands in deterministic model-lane order: each spans the offset range its
  // clusters occupy, with a little padding handled by the presentation.
  const lanes: LaneLayout[] = [];
  for (const lane of model.lanes) {
    const range = laneRange.get(lane.key);
    if (!range) {
      continue;
    }
    const rows = range.hi - range.lo + 1;
    const y = hubSpineY + range.lo * ROW_PITCH - NODE_H / 2;
    const height = rows * NODE_H + (rows - 1) * V_GAP;
    lanes.push({
      key: lane.key,
      hostsHub: lane.hostsHub,
      provider: lane.provider,
      y,
      height,
      centerY: y + height / 2,
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
  // Content is symmetric about the spine (`half` rows each side), so the bottom
  // mirrors the top margin's reserved rows.
  const contentBottom = hubSpineY + half * ROW_PITCH + NODE_H / 2;
  const height = contentBottom + MARGIN_BOTTOM;

  return {
    nodes,
    columns,
    bands,
    lanes,
    gates,
    hubSpineY,
    width,
    height,
    nodeW: NODE_W,
    nodeH: NODE_H,
  };
}
