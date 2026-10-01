import { identiconTiles } from "../../../domain/map/identicon";
import { ICON_BG } from "./colors";

export interface IdenticonProps {
  readonly seed: string;
  readonly x: number;
  readonly y: number;
  readonly size: number;
}

export function Identicon({
  seed,
  x,
  y,
  size,
}: IdenticonProps): React.ReactElement {
  const tiles = identiconTiles(seed, size);
  return (
    <g aria-hidden="true">
      <rect x={x} y={y} width={size} height={size} rx={3} fill={ICON_BG} />
      {tiles.map((tile, i) => (
        <rect
          key={i}
          x={x + tile.x}
          y={y + tile.y}
          width={tile.size}
          height={tile.size}
          fill={tile.fill}
        />
      ))}
    </g>
  );
}
