import {
  otherGateways,
  phaseCount,
  totalGateways,
  type GatewayPhaseCounts,
} from "../../../domain/fleet";
import { GATEWAY_COLOR } from "./colors";
import { f } from "./svg";

export interface GatewayDonutProps {
  readonly counts: GatewayPhaseCounts;
  readonly cx: number;
  readonly cy: number;
  readonly radius: number; // outer radius
}

export function GatewayDonut({
  counts,
  cx,
  cy,
  radius,
}: GatewayDonutProps): React.ReactElement {
  const total = totalGateways(counts);
  const strokeWidth = radius * 0.42;
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

  for (const { count, color } of segments) {
    if (count === 0) continue;
    const frac = count / total;
    const offset = cumulative;
    circles.push(
      <circle
        key={color}
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
