// The promotion fly-in overlay: the map's headliner animation. For each active flight
// (a bundle that just landed on a downstream node - see use-flights/computeFlights) it
// launches a copy of that bundle's identicon on an arc from the SOURCE node's identicon
// to the DEST node's, then bursts a little "dust" where it lands. The flying copy comes
// to rest exactly over the dest card's own (now-updated) identicon, so it reads as the
// bundle slotting home. Pure presentation: all timing/which-flights lives upstream.

import type { NodeBox } from "../../../domain/map/layout";
import { TONE_COLOR } from "./colors";
import { Identicon } from "./identicon";
import { f } from "./svg";
import styles from "./topology-map.module.css";
import type { ActiveFlight } from "./use-flights";

const SIZE = 30;
// Identicon center within its card: drawn at box.(x+11, y+11), size 30 -> +15 to centre.
const ICON_CENTER = 26;
// Number of arc samples handed to the travel keyframe. Denser = smoother curve; with
// linear per-segment timing the acceleration comes from the non-uniform t spacing.
const STEPS = 11;

// Dust specks fired outward from the landing point (slight upward bias). Precomputed as
// CSS-length strings so no number ever lands in a template literal (eslint).
const DUST = Array.from({ length: 10 }, (_, i) => {
  const a = (i / 10) * Math.PI * 2;
  const reach = 18 + (i % 3) * 7;
  return {
    dx: (Math.cos(a) * reach).toFixed(1) + "px",
    dy: (Math.sin(a) * reach - 5).toFixed(1) + "px",
  };
});

interface FlightArcProps {
  readonly seed: string;
  readonly sx: number;
  readonly sy: number;
  readonly dx: number;
  readonly dy: number;
  /** Quadratic bezier control point (shape of the arc). */
  readonly cx: number;
  readonly cy: number;
  /** Outline colour of the flying card - the dest's resting promotion-ring colour, so the
   *  outline doesn't change colour when the copy lands and the real card takes over. */
  readonly ring: string;
}

/** Point on the quadratic bezier P0->C->P2 at parameter t in [0,1]. */
function bezier(t: number, p0: number, c: number, p2: number): number {
  const u = 1 - t;
  return u * u * p0 + 2 * u * t * c + t * t * p2;
}

function FlightArc({
  seed,
  sx,
  sy,
  dx,
  dy,
  cx,
  cy,
  ring,
}: FlightArcProps): React.ReactElement {
  // Sample the arc at STEPS points for a CSS transform keyframe (not SMIL: a
  // dynamically inserted <animateMotion begin=0> on a long-lived SVG starts in the past
  // and snaps to its end). The t spacing is eased (t = p^1.7) so the bundle starts slow
  // and ACCELERATES into the landing, while linear per-segment timing keeps the dense
  // samples from stuttering - together a smooth "thrown through the air" arc.
  const vars: Record<string, string> = {};
  Array.from({ length: STEPS }, (_, i) => i).forEach((i) => {
    const t = Math.pow(i / (STEPS - 1), 1.7);
    vars["--f" + String(i) + "x"] = bezier(t, sx, cx, dx).toFixed(1) + "px";
    vars["--f" + String(i) + "y"] = bezier(t, sy, cy, dy).toFixed(1) + "px";
  });

  return (
    <g aria-hidden="true">
      {/* Two nested groups so position and scale don't fight over `transform`: the OUTER
          group carries the identicon along the arc (translate keyframe); the INNER group
          swells it larger mid-flight then settles to 1:1 on the dest card (scale). */}
      <g className={styles.flightTravel} style={vars}>
        <g className={styles.flightScale}>
          <rect
            x={-SIZE / 2 - 3}
            y={-SIZE / 2 - 3}
            width={SIZE + 6}
            height={SIZE + 6}
            rx={7}
            fill="none"
            stroke={ring}
            strokeWidth={3}
          />
          <Identicon seed={seed} x={-SIZE / 2} y={-SIZE / 2} size={SIZE} />
        </g>
      </g>

      {/* dust + impact ring at the landing site, delayed (in CSS) until the arc lands. */}
      <g transform={`translate(${f(dx)} ${f(dy)})`}>
        <circle
          className={styles.impactRing}
          cx={0}
          cy={0}
          r={10}
          fill="none"
          stroke="#e4d7bf"
          strokeWidth={2}
        />
        {DUST.map((p, i) => (
          <circle
            key={i}
            className={styles.dust}
            cx={0}
            cy={0}
            r={3}
            fill="#cbb79a"
            style={
              { "--dust-dx": p.dx, "--dust-dy": p.dy } as React.CSSProperties
            }
          />
        ))}
      </g>
    </g>
  );
}

export interface MapFlightsProps {
  readonly flights: readonly ActiveFlight[];
  readonly boxById: ReadonlyMap<string, NodeBox>;
  /** destId -> resting promotion-ring colour, so a flying copy's outline matches the card
   *  it lands on (no colour change on landing). */
  readonly ringByNodeId: ReadonlyMap<string, string>;
  /** The "CHANGE" diamond's launch point (its right tip): a brand-new bundle flies in
   *  from here into the first column, so the chain starts on-canvas at the change head
   *  (we can't fly it in from the off-canvas freight bar above). */
  readonly changeAnchor: { readonly x: number; readonly y: number };
}

/** Lofted arc control point for a source->dest hop (apex above the higher endpoint,
 *  taller for longer hops). */
function arcControl(
  sx: number,
  sy: number,
  dx: number,
  dy: number,
): { cx: number; cy: number } {
  return {
    cx: (sx + dx) / 2,
    cy: Math.min(sy, dy) - Math.max(80, Math.abs(dx - sx) * 0.35),
  };
}

export function MapFlights({
  flights,
  boxById,
  ringByNodeId,
  changeAnchor,
}: MapFlightsProps): React.ReactElement {
  return (
    <g aria-hidden="true" pointerEvents="none">
      {flights.map((flight) => {
        const d = boxById.get(flight.destId);
        if (!d) return null;
        const dx = d.x + ICON_CENTER;
        const dy = d.y + ICON_CENTER;

        // Source: the change diamond for a brand-new first-column bundle, else the
        // upstream node it promoted from.
        let sx: number;
        let sy: number;
        if (flight.fromTray) {
          sx = changeAnchor.x;
          sy = changeAnchor.y;
        } else {
          const s = boxById.get(flight.sourceId);
          if (!s) return null;
          sx = s.x + ICON_CENTER;
          sy = s.y + ICON_CENTER;
        }

        const { cx, cy } = arcControl(sx, sy, dx, dy);
        return (
          <FlightArc
            key={flight.key}
            seed={flight.seed}
            sx={sx}
            sy={sy}
            dx={dx}
            dy={dy}
            cx={cx}
            cy={cy}
            ring={ringByNodeId.get(flight.destId) ?? TONE_COLOR.info}
          />
        );
      })}
    </g>
  );
}
