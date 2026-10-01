import type { GatewayHistorySample } from "../../../domain/fleet";
import { GATEWAY_COLOR } from "./colors";

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
 */
export function SandSparkline({
  history,
  x,
  y,
  width,
  height,
}: SandSparklineProps): React.ReactElement | null {
  if (history.length < 2) return null;

  const n = history.length;
  const totals = history.map((s) => s.running + s.provisioning + s.failed);
  const max = Math.max(1, ...totals);

  const xAt = (i: number): number => x + (i / (n - 1)) * width;
  const yAt = (v: number): number => y + height - (v / max) * height;

  // Build each band as an area between its running lower and upper cumulative edge.
  const bands: React.ReactElement[] = [];
  let lower = history.map(() => 0);
  for (const { key, color } of LAYERS) {
    const upper = history.map((s, i) => (lower[i] ?? 0) + s[key]);
    const top = upper.map(
      (v, i) => `${xAt(i).toFixed(2)} ${yAt(v).toFixed(2)}`,
    );
    const bottom = lower
      .map((v, i) => `${xAt(i).toFixed(2)} ${yAt(v).toFixed(2)}`)
      .reverse();
    const d = `M ${top.join(" L ")} L ${bottom.join(" L ")} Z`;
    bands.push(
      <path key={color} d={d} fill={color} fillOpacity={0.82} stroke="none" />,
    );
    lower = upper;
  }

  return (
    <g aria-hidden="true" pointerEvents="none">
      {bands}
    </g>
  );
}
