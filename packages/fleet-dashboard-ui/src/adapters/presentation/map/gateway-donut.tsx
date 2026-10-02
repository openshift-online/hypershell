import {
  otherGateways,
  phaseCount,
  totalGateways,
  type GatewayPhaseCounts,
} from "../../../domain/fleet";
import { GATEWAY_COLOR } from "./colors";
import styles from "./gateway-donut.module.css";
import { f } from "./svg";

export interface GatewayDonutProps {
  readonly counts: GatewayPhaseCounts;
  readonly cx: number;
  readonly cy: number;
  readonly radius: number; // outer radius
}

/** Centre total, wrapped so a key={total} remount replays the pulse when it changes. */
function DonutTotal({
  total,
  cx,
  cy,
  radius,
}: {
  total: number;
  cx: number;
  cy: number;
  radius: number;
}): React.ReactElement {
  return (
    <g key={total} className={styles.total}>
      <text
        x={cx}
        y={cy}
        textAnchor="middle"
        dominantBaseline="central"
        fontSize={radius * 0.8}
        fontWeight="700"
        fill="currentColor"
      >
        {total}
      </text>
    </g>
  );
}

export function GatewayDonut({
  counts,
  cx,
  cy,
  radius,
}: GatewayDonutProps): React.ReactElement {
  const total = totalGateways(counts);
  // A slimmer ring (was 0.42) so the donut reads as a thin arc, not a fat band.
  const strokeWidth = radius * 0.26;
  const r = radius - strokeWidth / 2;
  const C = 2 * Math.PI * r;

  if (total === 0) {
    return (
      <g aria-hidden="true">
        <circle
          cx={cx}
          cy={cy}
          r={r}
          fill="none"
          stroke={GATEWAY_COLOR.idle}
          strokeWidth={strokeWidth}
        />
        <DonutTotal total={total} cx={cx} cy={cy} radius={radius} />
      </g>
    );
  }

  const segments: { count: number; color: string }[] = [
    { count: phaseCount(counts, "running"), color: GATEWAY_COLOR.running },
    {
      count: phaseCount(counts, "provisioning"),
      color: GATEWAY_COLOR.provisioning,
    },
    { count: phaseCount(counts, "failed"), color: GATEWAY_COLOR.failed },
    // Any phase the controller reports beyond the three above, so the ring closes
    // and the centre total always equals the sum of the drawn segments.
    { count: otherGateways(counts), color: GATEWAY_COLOR.idle },
  ];

  let cumulative = 0;
  const circles: React.ReactElement[] = [];

  // Every segment is rendered (zero-count ones as a zero-length dash) and keyed by colour,
  // so the SAME <circle> persists across count changes and the .seg CSS transition can
  // ease each arc smoothly to its new size/position instead of popping.
  for (const { count, color } of segments) {
    const frac = total > 0 ? count / total : 0;
    const offset = cumulative;
    circles.push(
      <circle
        key={color}
        className={styles.seg}
        cx={cx}
        cy={cy}
        r={r}
        fill="none"
        stroke={color}
        strokeWidth={strokeWidth}
        strokeDasharray={`${f(frac * C)} ${f(C)}`}
        strokeDashoffset={-offset * C}
      />,
    );
    cumulative += frac;
  }

  return (
    <g aria-hidden="true">
      <g transform={`rotate(-90 ${f(cx)} ${f(cy)})`}>{circles}</g>
      <DonutTotal total={total} cx={cx} cy={cy} radius={radius} />
    </g>
  );
}
