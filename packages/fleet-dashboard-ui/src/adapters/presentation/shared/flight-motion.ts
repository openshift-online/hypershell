// Shared flight motion primitive (SDLC-VIS-12a): the single source of the promotion
// flight's arc path, impact "dust" burst, and timing, so the Fleet Map (SVG, flying an
// identicon) and the Agent SDLC board (DOM, flying a full card) read as one motion
// language instead of each forking their own. Pure math + constants only; the matching
// CSS keyframes live beside this in flight-motion.module.css, and the per-surface
// overlays supply their own source/dest geometry.

/** Arc travel time; must stay in sync with the overlay CSS .flightTravel/.flightScale
 *  durations in flight-motion.module.css. */
export const FLIGHT_MS = 900;
/** Dust + impact ring linger past landing before the flying copy is removed (their CSS
 *  delay ~matches FLIGHT_MS, then they play ~0.65s). */
export const DUST_MS = 800;

// Number of arc samples handed to the travel keyframe (matches the --f0..--f10 slots in
// the CSS). Denser = smoother curve; with linear per-segment timing the acceleration
// comes from the non-uniform t spacing.
const ARC_STEPS = 11;

/** Dust specks fired outward from the landing point (slight upward bias). Precomputed as
 *  CSS-length strings so no number ever lands in a template literal (eslint). */
export const DUST: readonly { readonly dx: string; readonly dy: string }[] =
  Array.from({ length: 10 }, (_, i) => {
    const a = (i / 10) * Math.PI * 2;
    const reach = 18 + (i % 3) * 7;
    return {
      dx: (Math.cos(a) * reach).toFixed(1) + "px",
      dy: (Math.sin(a) * reach - 5).toFixed(1) + "px",
    };
  });

/** True when the user asked the platform to minimise motion. Flights are suppressed
 *  entirely in that case (accessibility.spec); callers snap to the new state instead. */
export function prefersReducedMotion(): boolean {
  return (
    typeof window !== "undefined" &&
    typeof window.matchMedia === "function" &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches
  );
}

/** Point on the quadratic bezier P0->C->P2 at parameter t in [0,1]. */
function bezier(t: number, p0: number, c: number, p2: number): number {
  const u = 1 - t;
  return u * u * p0 + 2 * u * t * c + t * t * p2;
}

/** Lofted arc control point for a source->dest hop (apex above the higher endpoint,
 *  taller for longer hops). Shared so both surfaces arc with the same shape. */
export function arcControl(
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

/**
 * Sample the arc at ARC_STEPS points as a CSS custom-property set (--f0x..--f10y) for
 * the .flightTravel keyframe. Not SMIL: a dynamically inserted animateMotion on a
 * long-lived SVG starts in the past and snaps to its end. The t spacing is eased
 * (t = p^1.7) so the copy starts slow and ACCELERATES into the landing, while linear
 * per-segment CSS timing keeps the dense samples from stuttering - together a smooth
 * "thrown through the air" arc. Coordinates are whatever space the caller works in: the
 * map passes absolute SVG coords; the board passes source-relative translate deltas
 * (source at 0,0).
 */
export function arcTravelVars(
  sx: number,
  sy: number,
  dx: number,
  dy: number,
  cx: number,
  cy: number,
): Record<string, string> {
  const vars: Record<string, string> = {};
  Array.from({ length: ARC_STEPS }, (_, i) => i).forEach((i) => {
    const t = Math.pow(i / (ARC_STEPS - 1), 1.7);
    vars["--f" + String(i) + "x"] = bezier(t, sx, cx, dx).toFixed(1) + "px";
    vars["--f" + String(i) + "y"] = bezier(t, sy, cy, dy).toFixed(1) + "px";
  });
  return vars;
}
