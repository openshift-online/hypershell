import { initialsDataUri } from "../shared/initials";
import { ICON_BG } from "./colors";

export interface IdenticonProps {
  readonly seed: string;
  readonly x: number;
  readonly y: number;
  readonly size: number;
}

// A bundle identicon inside an SVG: the DiceBear Initials picture for the seed
// (FLT-DLV-24). Decorative; the alias next to it carries the meaning.
export function Identicon({
  seed,
  x,
  y,
  size,
}: IdenticonProps): React.ReactElement {
  return (
    <g aria-hidden="true">
      <rect x={x} y={y} width={size} height={size} rx={3} fill={ICON_BG} />
      <image
        data-identicon-seed={seed}
        height={size}
        href={initialsDataUri(seed)}
        preserveAspectRatio="xMidYMid meet"
        width={size}
        x={x}
        y={y}
      />
    </g>
  );
}
