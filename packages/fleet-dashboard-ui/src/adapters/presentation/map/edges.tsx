// The promotion "rails": cubic-bezier edges that funnel every node in a column
// into that column's gate diamond (to its right), then fan back out into the next
// column's nodes. The terminal gate (final stage) only funnels in - there is no
// downstream column. Gates are diamonds tinted by their badge tone (color is
// paired with the drawer's text label, never the sole signal). Purely geometric -
// it consumes the model (column membership) and the computed layout (coordinates).

import type { MapModel } from "../../../domain/map/model";
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
    // A dark disc with a white spinner, so it stays legible on the amber diamond
    // (a tinted spinner on amber washed out - AAA contrast).
    return (
      <g transform={`translate(${f(cx)}, ${f(cy)})`} aria-hidden="true">
        <circle r={8} fill="#10202f" />
        <circle
          r={5.5}
          fill="none"
          stroke="#ffffff"
          strokeOpacity={0.3}
          strokeWidth={2}
        />
        <circle
          r={5.5}
          fill="none"
          stroke="#ffffff"
          strokeWidth={2}
          strokeLinecap="round"
          strokeDasharray="9 100"
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
      <defs>
        {/* Arrowheads for the promoting rails. The active (inbound) head inherits
            the path's tone via context-stroke; the outbound head is fixed grey. */}
        <marker
          id="gate-arrow-active"
          viewBox="0 0 10 10"
          refX="8"
          refY="5"
          markerWidth="6"
          markerHeight="6"
          orient="auto"
        >
          <path d="M0 0 L10 5 L0 10 z" fill="context-stroke" />
        </marker>
        <marker
          id="gate-arrow-grey"
          viewBox="0 0 10 10"
          refX="8"
          refY="5"
          markerWidth="6"
          markerHeight="6"
          orient="auto"
        >
          <path d="M0 0 L10 5 L0 10 z" fill={EDGE_STROKE} />
        </marker>
      </defs>
      {model.gates.map((gate) => {
        const gl = gateById.get(gate.id);
        if (!gl) {
          return null;
        }
        // Three rail states, in priority order:
        //  - passed  (gate tone success): promotion completed, so BOTH sides are
        //    solid green with arrowheads - the flow has carried through to the next
        //    cluster. No dashes, no animation.
        //  - promoting: dashed "marching ants" arrows (tinted by the gate tone) flow
        //    INTO the gate to show the condition being evaluated, and a static grey
        //    arrow points on to the next cluster (no real flow yet - it is blocked
        //    until the gate passes).
        //  - settled/other: plain thin rails.
        const passed = gate.badge.tone === "success";
        const promoting = gate.promoting && !passed;
        const fromIds = colNodeIds.get(gate.fromColumnKey) ?? [];
        const toIds = colNodeIds.get(gate.toColumnKey) ?? [];
        const edges: React.ReactElement[] = [];
        const passedRail = (key: string, d: string): React.ReactElement => (
          <path
            key={key}
            d={d}
            fill="none"
            stroke={TONE_COLOR.success}
            strokeWidth={2}
            strokeOpacity={0.9}
            markerEnd="url(#gate-arrow-active)"
          />
        );
        const plainRail = (key: string, d: string): React.ReactElement => (
          <path
            key={key}
            d={d}
            fill="none"
            stroke={EDGE_STROKE}
            strokeWidth={1.25}
            strokeOpacity={0.7}
          />
        );
        for (const id of fromIds) {
          const b = boxById.get(id);
          if (!b) {
            continue;
          }
          const d = hBezier(b.x + b.w, b.cy, gl.x, gl.y);
          const key = `${gate.id}:in:${id}`;
          if (passed) {
            edges.push(passedRail(key, d));
          } else if (promoting) {
            edges.push(
              <path
                key={key}
                d={d}
                fill="none"
                stroke={TONE_COLOR[gate.badge.tone]}
                strokeWidth={2}
                strokeOpacity={0.9}
                strokeDasharray="6 5"
                markerEnd="url(#gate-arrow-active)"
              >
                <animate
                  attributeName="stroke-dashoffset"
                  from="22"
                  to="0"
                  dur="0.8s"
                  repeatCount="indefinite"
                />
              </path>,
            );
          } else {
            edges.push(plainRail(key, d));
          }
        }
        for (const id of toIds) {
          const b = boxById.get(id);
          if (!b) {
            continue;
          }
          const d = hBezier(gl.x, gl.y, b.x, b.cy);
          const key = `${gate.id}:out:${id}`;
          if (passed) {
            edges.push(passedRail(key, d));
          } else if (promoting) {
            edges.push(
              <path
                key={key}
                d={d}
                fill="none"
                stroke={EDGE_STROKE}
                strokeWidth={1.5}
                strokeOpacity={0.7}
                markerEnd="url(#gate-arrow-grey)"
              />,
            );
          } else {
            edges.push(plainRail(key, d));
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
            />
            <GateGlyph tone={gate.badge.tone} cx={gl.x} cy={gl.y} />
            {/* gate name BELOW the diamond on the dark canvas (not cramped onto the
                tinted fill, which failed AAA contrast and overflowed the glyph) */}
            {gate.name ? (
              <text
                x={gl.x}
                y={gl.y + GATE_R + 14}
                textAnchor="middle"
                fontSize={10}
                fontWeight={700}
                fill={TEXT_COLOR}
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
