import { useId } from "react";

import { SANDBOX_COLOR } from "./colors";
import styles from "./topology-map.module.css";

export interface SandboxSparklineProps {
  readonly history: readonly number[];
  readonly x: number;
  readonly y: number;
  readonly width: number;
  readonly height: number;
}

/**
 * Per-instance active-sandbox history as a single-band "sand" chart - the node card's
 * LOWER chin, sitting directly below the gateway sand sparkline. Sandboxes are one
 * population (unlike gateways' running/provisioning/failed), so this is one teal band
 * rather than a stack. It is drawn on the SAME 24h/32-sample grid and at the SAME x /
 * width as the gateway sparkline, so the two chins line up and are directly comparable
 * (a sandbox wave reads against the gateway wave at the same point in time).
 *
 * CONVEYOR: identical mechanics to SandSparkline - history is a sliding window (newest
 * on the right), so a fresh sample slides the whole series LEFT by one slot. A phantom
 * slot one step off the left keeps the slide gap-free, a clip box hides the spill, and
 * the parent remounts this subtree on each new sample to replay the slide. Collapses to
 * a static chart under prefers-reduced-motion (CSS).
 *
 * Decorative (aria-hidden): the accessible, translated equivalent is the sandbox widget
 * + per-cluster breakdown in the detail drawer. Renders nothing until there are at
 * least two samples to draw a band between, keeping fresh/idle cards clean.
 */
export function SandboxSparkline({
  history,
  x,
  y,
  width,
  height,
}: SandboxSparklineProps): React.ReactElement | null {
  const clipId = useId();
  if (history.length < 2) return null;

  const n = history.length;
  const step = width / (n - 1);
  const max = Math.max(1, ...history);

  // Index -1 is a phantom slot duplicating the oldest sample, one step LEFT of the
  // chart so the leftward slide has content to reveal (no gap). Real samples map
  // 0..n-1 across the width; the newest (n-1) sits exactly on the right edge.
  const indices = [-1, ...history.map((_, i) => i)];
  const sampleAt = (i: number): number => history[Math.max(0, i)] ?? 0;
  const xAt = (i: number): number => x + i * step;
  const yAt = (v: number): number => y + height - (v / max) * height;

  const top = indices.map(
    (idx) => `${xAt(idx).toFixed(2)} ${yAt(sampleAt(idx)).toFixed(2)}`,
  );
  const bottom = indices
    .map((idx) => `${xAt(idx).toFixed(2)} ${yAt(0).toFixed(2)}`)
    .reverse();
  const d = `M ${top.join(" L ")} L ${bottom.join(" L ")} Z`;

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
          <path d={d} fill={SANDBOX_COLOR} fillOpacity={0.82} stroke="none" />
        </g>
      </g>
    </g>
  );
}
