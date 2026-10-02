// Map LAYOUT: pure geometry over the MapModel. Given columns (promotion order),
// lanes (hub spine + provider spokes) and nodes, it computes absolute SVG
// coordinates. No DOM, no measurement - deterministic from the model alone so it
// is unit-testable and the presentation layer just renders the boxes it returns.

import type { MapColumn, MapModel, MapNode } from "./model";

/** Fixed card + spacing constants (SVG user units). Tuned to the prototype. */
export const NODE_W = 150;
export const NODE_H = 106;
export const COL_GAP = 190;
export const V_GAP = 46;
export const LANE_GAP = 46;
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
  readonly lanes: readonly LaneLayout[];
  readonly gates: readonly GateLayout[];
  /** Y of the promotion spine (hub row): gates + the change diamond ride this. */
  readonly hubSpineY: number;
  readonly width: number;
  readonly height: number;
  readonly nodeW: number;
  readonly nodeH: number;
}

function laneHeight(stack: number): number {
  const n = Math.max(1, stack);
  return n * NODE_H + (n - 1) * V_GAP;
}

/**
 * Compute absolute positions for every node, column, lane and gate. Columns are
 * evenly spaced left -> right in model order; lanes stack top -> bottom (the hub
 * spine already centred by the model's lane ordering). Multiple nodes sharing one
 * (column, lane) cell stack vertically within that lane.
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

  // How many nodes land in each (lane, column) cell -> lane row heights.
  const nodesByLane = new Map<string, MapNode[]>();
  for (const n of model.nodes) {
    const bucket = nodesByLane.get(n.laneKey);
    if (bucket) {
      bucket.push(n);
    } else {
      nodesByLane.set(n.laneKey, [n]);
    }
  }
  const maxStackPerLane = new Map<string, number>();
  for (const [laneKey, laneNodes] of nodesByLane) {
    const perColumn = new Map<string, number>();
    for (const n of laneNodes) {
      perColumn.set(n.columnKey, (perColumn.get(n.columnKey) ?? 0) + 1);
    }
    maxStackPerLane.set(laneKey, Math.max(1, ...perColumn.values()));
  }

  // Lay lanes top -> bottom in the model's lane order.
  const lanes: LaneLayout[] = [];
  let runningY = MARGIN_TOP;
  for (const lane of model.lanes) {
    const h = laneHeight(maxStackPerLane.get(lane.key) ?? 1);
    lanes.push({
      key: lane.key,
      hostsHub: lane.hostsHub,
      provider: lane.provider,
      y: runningY,
      height: h,
      centerY: runningY + h / 2,
    });
    runningY += h + LANE_GAP;
  }
  const laneByKey = new Map(lanes.map((l) => [l.key, l]));

  // Place nodes; stack within a (column, lane) cell as they appear in the model.
  const stackIndex = new Map<string, number>();
  const nodes: NodeBox[] = [];
  for (const n of model.nodes) {
    const col = colByKey.get(n.columnKey);
    const lane = laneByKey.get(n.laneKey);
    if (!col || !lane) {
      continue;
    }
    const cellKey = `${n.laneKey}|${n.columnKey}`;
    const si = stackIndex.get(cellKey) ?? 0;
    stackIndex.set(cellKey, si + 1);
    const x = col.x;
    const y = lane.y + si * (NODE_H + V_GAP);
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

  // Gates ride the promotion spine = the hub row, which sits at the TOP of the
  // hub-hosting cloud's lane (one hub per env, sub-stacked at index 0). Fallback
  // to the vertical midpoint when no lane hosts a hub.
  const hubLane = lanes.find((l) => l.hostsHub);
  const gateY = hubLane
    ? hubLane.y + NODE_H / 2
    : (MARGIN_TOP + Math.max(runningY - LANE_GAP, MARGIN_TOP)) / 2;
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
      y: gateY,
      fromColumnKey: g.fromColumnKey,
      toColumnKey: g.toColumnKey,
    });
  }

  const lastCol = columns[columns.length - 1];
  const width = lastCol ? lastCol.x + NODE_W + MARGIN_X : MARGIN_X * 2 + NODE_W;
  const height = Math.max(runningY - LANE_GAP, MARGIN_TOP) + MARGIN_BOTTOM;

  return {
    nodes,
    columns,
    bands,
    lanes,
    gates,
    hubSpineY: gateY,
    width,
    height,
    nodeW: NODE_W,
    nodeH: NODE_H,
  };
}
