import type { SandboxClusterCount } from "../../../domain/fleet";
import { SANDBOX_COLOR, SANDBOX_TRACK } from "./colors";

export interface SandboxStripProps {
  readonly clusters: readonly SandboxClusterCount[];
  readonly x: number;
  readonly y: number;
  readonly width: number;
  readonly height: number;
}

const BAR_GAP = 2;

/**
 * Per-cluster active-sandbox population as a tiny column chart - the node card's
 * SECOND "chin", sitting directly below the gateway "sand" sparkline. One teal bar
 * per managed cluster that currently hosts sandboxes, scaled to the busiest cluster
 * on THIS node so the bars are comparable within the card (the same within-node scale
 * the sand sparkline uses). A faint full-width track shows the chart's extent even
 * when only one cluster is populated.
 *
 * Decorative (aria-hidden): the accessible, translated equivalent is the sandbox
 * widget + per-cluster breakdown in the detail drawer. Renders nothing when no
 * cluster on this node hosts a sandbox, keeping idle cards clean.
 */
export function SandboxStrip({
  clusters,
  x,
  y,
  width,
  height,
}: SandboxStripProps): React.ReactElement | null {
  const shown = clusters.filter((c) => c.count > 0);
  if (shown.length === 0) return null;

  const n = shown.length;
  const max = Math.max(1, ...shown.map((c) => c.count));
  const barW = Math.max(1, (width - (n - 1) * BAR_GAP) / n);

  return (
    <g aria-hidden="true" pointerEvents="none">
      {/* track: the chart box, so a single populated cluster still reads as a chart */}
      <rect
        x={x}
        y={y}
        width={width}
        height={height}
        rx={2}
        fill={SANDBOX_TRACK}
      />
      {shown.map((c, i) => {
        // Every populated cluster shows at least 1px so a non-zero count is never
        // invisible; the busiest fills the full strip height.
        const barH = Math.max(1, (c.count / max) * height);
        const bx = x + i * (barW + BAR_GAP);
        return (
          <rect
            key={c.cluster}
            x={bx}
            y={y + height - barH}
            width={barW}
            height={barH}
            rx={1}
            fill={SANDBOX_COLOR}
            fillOpacity={0.85}
          />
        );
      })}
    </g>
  );
}
