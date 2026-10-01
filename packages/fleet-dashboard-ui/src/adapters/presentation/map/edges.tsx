// The promotion "rails": cubic-bezier edges that funnel every node in a column
// into the gate diamond between it and the next column, then fan back out into the
// next column's nodes. Gates are diamonds tinted by their badge tone (color is
// paired with the drawer's text label, never the sole signal). Purely geometric -
// it consumes the model (column membership) and the computed layout (coordinates).

import type { MapModel } from "../../../domain/map/model";
import type {
  GateLayout,
  MapLayout,
  NodeBox,
} from "../../../domain/map/layout";
import { EDGE_STROKE, TONE_COLOR } from "./colors";
import { f, pt } from "./svg";

const GATE_R = 11;

/** Cubic bezier between two points with horizontal control handles. */
function hBezier(x1: number, y1: number, x2: number, y2: number): string {
  const midX = (x1 + x2) / 2;
  return `M ${f(x1)} ${f(y1)} C ${f(midX)} ${f(y1)}, ${f(midX)} ${f(y2)}, ${f(x2)} ${f(y2)}`;
}

export interface MapEdgesProps {
  readonly model: MapModel;
  readonly layout: MapLayout;
  readonly selectedGateId: string | null;
  readonly onSelectGate: (id: string) => void;
}

export function MapEdges({
  model,
  layout,
  selectedGateId,
  onSelectGate,
}: MapEdgesProps): React.ReactElement {
  const boxById = new Map<string, NodeBox>(layout.nodes.map((b) => [b.id, b]));
  const colNodeIds = new Map<string, readonly string[]>(
    model.columns.map((c) => [c.key, c.nodeIds]),
  );
  const gateById = new Map<string, GateLayout>(
    layout.gates.map((g) => [g.id, g]),
  );

  return (
    <g>
      {model.gates.map((gate) => {
        const gl = gateById.get(gate.id);
        if (!gl) {
          return null;
        }
        const tone = gate.promoting ? TONE_COLOR[gate.badge.tone] : EDGE_STROKE;
        const fromIds = colNodeIds.get(gate.fromColumnKey) ?? [];
        const toIds = colNodeIds.get(gate.toColumnKey) ?? [];
        const edges: React.ReactElement[] = [];
        for (const id of fromIds) {
          const b = boxById.get(id);
          if (b) {
            edges.push(
              <path
                key={`${gate.id}:in:${id}`}
                d={hBezier(b.x + b.w, b.cy, gl.x, gl.y)}
                fill="none"
                stroke={tone}
                strokeWidth={gate.promoting ? 2 : 1.25}
                strokeOpacity={0.7}
              />,
            );
          }
        }
        for (const id of toIds) {
          const b = boxById.get(id);
          if (b) {
            edges.push(
              <path
                key={`${gate.id}:out:${id}`}
                d={hBezier(gl.x, gl.y, b.x, b.cy)}
                fill="none"
                stroke={tone}
                strokeWidth={gate.promoting ? 2 : 1.25}
                strokeOpacity={0.7}
              />,
            );
          }
        }
        return <g key={gate.id}>{edges}</g>;
      })}

      {/* gate diamonds (drawn above the rails) */}
      {model.gates.map((gate) => {
        const gl = gateById.get(gate.id);
        if (!gl) {
          return null;
        }
        const fill = TONE_COLOR[gate.badge.tone];
        const selected = selectedGateId === gate.id;
        const select = () => {
          onSelectGate(gate.id);
        };
        const onKeyDown = (e: React.KeyboardEvent) => {
          if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            select();
          }
        };
        const pts = [
          pt(gl.x, gl.y - GATE_R),
          pt(gl.x + GATE_R, gl.y),
          pt(gl.x, gl.y + GATE_R),
          pt(gl.x - GATE_R, gl.y),
        ].join(" ");
        return (
          <polygon
            key={gate.id}
            points={pts}
            fill={fill}
            stroke={selected ? "#ffffff" : "rgba(0,0,0,0.4)"}
            strokeWidth={selected ? 2.5 : 1}
            role="button"
            tabIndex={0}
            aria-label={gate.id}
            aria-pressed={selected}
            onClick={select}
            onKeyDown={onKeyDown}
            style={{ cursor: "pointer" }}
          >
            {gate.promoting ? (
              <animate
                attributeName="opacity"
                values="1;0.45;1"
                dur="1.4s"
                repeatCount="indefinite"
              />
            ) : null}
          </polygon>
        );
      })}
    </g>
  );
}
