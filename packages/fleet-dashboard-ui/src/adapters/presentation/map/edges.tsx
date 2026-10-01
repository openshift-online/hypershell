// The promotion "rails": cubic-bezier edges that funnel every node in a column
// into that column's gate diamond (to its right), then fan back out into the next
// column's nodes. The terminal gate (final stage) only funnels in - there is no
// downstream column. Gates are diamonds tinted by their badge tone (color is
// paired with the drawer's text label, never the sole signal). Purely geometric -
// it consumes the model (column membership) and the computed layout (coordinates).

import type { MapGate, MapModel } from "../../../domain/map/model";
import type {
  GateLayout,
  MapLayout,
  NodeBox,
} from "../../../domain/map/layout";
import type { SemanticTone } from "../../../domain/status";
import { EDGE_STROKE, TEXT_COLOR, TEXT_SUBTLE, TONE_COLOR } from "./colors";
import { f, pt } from "./svg";

const GATE_R = 20;

/**
 * The status glyph that rides a gate diamond, centred on (cx, cy): a green
 * check for a passed gate, a red exclamation for a failed one, a spinner while
 * it waits. Info/unknown gates draw nothing. Colour is paired with the drawer's
 * translated label, so the glyph is never the sole signal.
 */
function GateGlyph({
  tone,
  cx,
  cy,
}: {
  tone: SemanticTone;
  cx: number;
  cy: number;
}): React.ReactElement | null {
  if (tone === "success") {
    return (
      <g transform={`translate(${f(cx)}, ${f(cy)})`} aria-hidden="true">
        <circle r={8} fill={TONE_COLOR.success} />
        <path
          d="M-3.6 0.2 L-1.1 2.7 L3.7 -2.9"
          fill="none"
          stroke="#ffffff"
          strokeWidth={1.8}
          strokeLinecap="round"
          strokeLinejoin="round"
        />
      </g>
    );
  }
  if (tone === "danger") {
    return (
      <g transform={`translate(${f(cx)}, ${f(cy)})`} aria-hidden="true">
        <circle r={8} fill={TONE_COLOR.danger} />
        <rect
          x={-0.9}
          y={-4.3}
          width={1.8}
          height={5.3}
          rx={0.9}
          fill="#ffffff"
        />
        <circle cx={0} cy={3.5} r={1.1} fill="#ffffff" />
      </g>
    );
  }
  if (tone === "warning") {
    return (
      <g transform={`translate(${f(cx)}, ${f(cy)})`} aria-hidden="true">
        <circle r={7} fill="#ffffff" opacity={0.75} />
        <circle
          r={7}
          fill="none"
          stroke={TONE_COLOR.info}
          strokeOpacity={0.25}
          strokeWidth={2}
        />
        <circle
          r={7}
          fill="none"
          stroke={TONE_COLOR.info}
          strokeWidth={2}
          strokeLinecap="round"
          strokeDasharray="13 100"
        >
          <animateTransform
            attributeName="transform"
            type="rotate"
            from="0"
            to="360"
            dur="1s"
            repeatCount="indefinite"
          />
        </circle>
      </g>
    );
  }
  return null;
}

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
        // Label above the diamond: the SOURCE env whose analysis this gate reports
        // (the condition to promote out of it). Data-derived, no fleet knowledge.
        const promoteLabel = gate.fromColumnKey;
        return (
          <g
            key={gate.id}
            role="button"
            tabIndex={0}
            aria-label={gate.id}
            aria-pressed={selected}
            onClick={select}
            onKeyDown={onKeyDown}
            style={{ cursor: "pointer" }}
          >
            <text
              x={gl.x}
              y={gl.y - GATE_R - 6}
              textAnchor="middle"
              fontSize={10}
              fill={TEXT_SUBTLE}
            >
              {promoteLabel}
            </text>
            <polygon
              points={pts}
              fill={fill}
              stroke={selected ? "#ffffff" : "rgba(0,0,0,0.4)"}
              strokeWidth={selected ? 2.5 : 1}
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
            <GateGlyph tone={gate.badge.tone} cx={gl.x} cy={gl.y - 7} />
            {/* gate name below the glyph, inside the diamond (server data) */}
            {gate.name ? (
              <text
                x={gl.x}
                y={gl.y + 9}
                textAnchor="middle"
                fontSize={10}
                fontWeight={700}
                fill={glyphLabelFill(gate)}
              >
                {gate.name}
              </text>
            ) : null}
          </g>
        );
      })}
    </g>
  );
}

/** The gate-name text sits on the diamond's fill; pick a legible ink for it. */
function glyphLabelFill(gate: MapGate): string {
  return gate.badge.tone === "unknown" || gate.badge.tone === "info"
    ? TEXT_COLOR
    : "#10202f";
}
