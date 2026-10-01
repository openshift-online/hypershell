import { SAND_FILL, SAND_LINE } from "./colors";

export interface SandSparklineProps {
  readonly history: readonly number[];
  readonly x: number;
  readonly y: number;
  readonly width: number;
  readonly height: number;
}

export function SandSparkline({
  history,
  x,
  y,
  width,
  height,
}: SandSparklineProps): React.ReactElement | null {
  if (history.length < 2) return null;

  const n = history.length;
  const min = Math.min(...history);
  const max = Math.max(...history);
  const flat = max === min;

  const points = history.map((v, i) => {
    const px = x + (i / (n - 1)) * width;
    const py = flat
      ? y + height / 2
      : y + height - ((v - min) / (max - min)) * height;
    return `${px.toFixed(2)},${py.toFixed(2)}`;
  });

  // Filled area polygon: bottom-left → all points → bottom-right → close
  const areaPoints = [
    `${x.toFixed(2)},${(y + height).toFixed(2)}`,
    ...points,
    `${(x + width).toFixed(2)},${(y + height).toFixed(2)}`,
  ].join(" ");

  const linePoints = points.join(" ");

  return (
    <g aria-hidden="true" pointerEvents="none">
      <polygon
        points={areaPoints}
        fill={SAND_FILL}
        fillOpacity={0.22}
        stroke="none"
      />
      <polyline
        points={linePoints}
        fill="none"
        stroke={SAND_LINE}
        strokeWidth={1.25}
        strokeLinejoin="round"
        strokeLinecap="round"
      />
    </g>
  );
}
