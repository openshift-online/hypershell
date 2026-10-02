// Identicon generation for HyperShell map markers. Pure, deterministic, and
// framework-free - all output is plain data suitable for SVG rendering. A given
// seed always produces the same glyph; no browser globals or randomness are used.

import { fnv1a } from "./hash";

/** Edge length (in cells) of the identicon grid. */
export const IDENTICON_GRID = 7;

/** Number of left-half columns generated before mirroring. */
const COLS = 4;

/** A single filled cell in the left half of the identicon grid. */
export interface IdenticonCell {
  readonly col: number;
  readonly row: number;
  /** When true, the cell uses `color2`; when false it uses `color`. */
  readonly alt: boolean;
}

/** Full identicon model derived from a seed string. */
export interface IdenticonModel {
  readonly color: string;
  readonly color2: string;
  /** Left-half cells only (col 0–3). Mirror to produce the full grid. */
  readonly cells: readonly IdenticonCell[];
}

/** A positioned, sized, coloured tile ready for SVG rendering. */
export interface IdenticonTile {
  readonly x: number;
  readonly y: number;
  readonly size: number;
  readonly fill: string;
}

/**
 * Derives a deterministic identicon model from `seed`. The model contains
 * the two accent colours and the left-half cell list; call `identiconTiles`
 * to convert it into SVG geometry.
 */
export function identiconModel(seed: string): IdenticonModel {
  const hue = fnv1a("hue:" + seed) % 360;
  const color = `hsl(${String(hue)}, 62%, 48%)`;
  const color2 = `hsl(${String((hue + 40) % 360)}, 55%, 62%)`;
  const density = 35 + (fnv1a("den:" + seed) % 38);

  const cells: IdenticonCell[] = [];
  for (let col = 0; col < COLS; col++) {
    for (let row = 0; row < IDENTICON_GRID; row++) {
      const h = fnv1a(`${String(col)}:${String(row)}:${seed}`);
      if (h % 100 < density) {
        cells.push({ col, row, alt: (h & 4) === 0 });
      }
    }
  }

  return { color, color2, cells };
}

/**
 * Converts a seed into a flat list of SVG-ready tiles for a square canvas of
 * `pixels` edge length. Each left-half cell is mirrored to the right; the
 * centre column (col 3, which maps to itself under 6-col) emits only one tile.
 */
export function identiconTiles(
  seed: string,
  pixels: number,
): readonly IdenticonTile[] {
  const model = identiconModel(seed);
  const cellSize = pixels / IDENTICON_GRID;
  const size = cellSize + 0.5;

  const tiles: IdenticonTile[] = [];
  for (const cell of model.cells) {
    const fill = cell.alt ? model.color2 : model.color;
    const mirrorCol = IDENTICON_GRID - 1 - cell.col;

    tiles.push({ x: cell.col * cellSize, y: cell.row * cellSize, size, fill });

    if (mirrorCol !== cell.col) {
      tiles.push({
        x: mirrorCol * cellSize,
        y: cell.row * cellSize,
        size,
        fill,
      });
    }
  }

  return tiles;
}
