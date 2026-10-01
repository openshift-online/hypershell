import { useId } from "react";

import type { GatewayHistorySample } from "../../../domain/fleet";
import { GATEWAY_COLOR } from "./colors";
import styles from "./topology-map.module.css";

export interface SandSparklineProps {
  readonly history: readonly GatewayHistorySample[];
  readonly x: number;
  readonly y: number;
  readonly width: number;
  readonly height: number;
}

// Stacking order bottom -> top, matching the prototype's three-layer "sand":
// running (green) at the base, then provisioning (amber), then failed (red).
const LAYERS: readonly { key: keyof GatewayHistorySample; color: string }[] = [
  { key: "running", color: GATEWAY_COLOR.running },
  { key: "provisioning", color: GATEWAY_COLOR.provisioning },
  { key: "failed", color: GATEWAY_COLOR.failed },
];

/**
 * Per-instance gateway history as a stacked area ("sand") chart: each phase is its
 * own filled band so a healthy fleet reads green, a provisioning wave rises amber,
 * and failures surface red on top. The y-scale is the max stacked total across the
 * window, so band heights are comparable within one node.
 *
 * CONVEYOR: history is a sliding window (newest on the right), so when a fresh sample
 * lands the whole series must appear to SHIFT LEFT by one slot - the newest band
 * sliding in from the right edge, the oldest sliding off the left - rather than the
 * chart re-drawing in place. We render one extra "phantom" slot off the left so the
 * slide never opens a gap, clip to the chart box so nothing spills past the edges,
 * and let React replay the slide by remounting this subtree on each new sample (the
 * parent keys it on the latest sample). The slide collapses to a static chart under
 * prefers-reduced-motion (CSS).
 */
export function SandSparkline({
  history,
  x,
  y,
  width,
  height,
}: SandSparklineProps): React.ReactElement | null {
  const clipId = useId();
  const first = history[0];
  if (history.length < 2 || !first) return null;

  const n = history.length;
  const step = width / (n - 1);
  const totals = history.map((s) => s.running + s.provisioning + s.failed);
  const max = Math.max(1, ...totals);

  // Index -1 is a phantom slot duplicating the oldest sample, placed one step LEFT of
  // the chart so the leftward slide has content to reveal (no gap). Real samples map
  // 0..n-1 across the chart width; the newest (n-1) sits exactly on the right edge.
  const indices = [-1, ...history.map((_, i) => i)];
  const sampleAt = (i: number): GatewayHistorySample =>
    history[Math.max(0, i)] ?? first;
  const xAt = (i: number): number => x + i * step;
  const yAt = (v: number): number => y + height - (v / max) * height;

  // Stack the layers bottom-up: each band fills between the running cumulative lower
  // edge and that edge plus this layer's own value.
  const bands: React.ReactElement[] = [];
  let lowerVals = indices.map(() => 0);
  for (const { key, color } of LAYERS) {
    const upperVals = indices.map(
      (idx, p) => (lowerVals[p] ?? 0) + sampleAt(idx)[key],
    );
    const top = indices.map(
      (idx, p) => `${xAt(idx).toFixed(2)} ${yAt(upperVals[p] ?? 0).toFixed(2)}`,
    );
    const bottom = indices
      .map(
        (idx, p) =>
          `${xAt(idx).toFixed(2)} ${yAt(lowerVals[p] ?? 0).toFixed(2)}`,
      )
      .reverse();
    const d = `M ${top.join(" L ")} L ${bottom.join(" L ")} Z`;
    bands.push(
      <path key={color} d={d} fill={color} fillOpacity={0.82} stroke="none" />,
    );
    lowerVals = upperVals;
  }

  return (
    <g aria-hidden="true" pointerEvents="none">
      <clipPath id={clipId}>
        <rect x={x} y={y - 2} width={width} height={height + 4} />
      </clipPath>
      {/* static clip window; the inner group slides left into it on each new sample */}
      <g clipPath={`url(#${clipId})`}>
        <g
          className={styles.sandShift}
          style={{ "--sand-step": String(step) + "px" } as React.CSSProperties}
        >
          {bands}
        </g>
      </g>
    </g>
  );
}
