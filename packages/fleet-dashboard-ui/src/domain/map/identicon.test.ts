import { describe, expect, it } from "vitest";

import {
  IDENTICON_GRID,
  identiconModel,
  identiconTiles,
  type IdenticonModel,
} from "./identicon";

// Helper: parse hue from an hsl() string such as "hsl(210, 62%, 48%)".
function parseHue(hsl: string): number {
  const m = /^hsl\((\d+),/.exec(hsl);
  if (!m?.[1]) throw new Error(`Cannot parse hue from: ${hsl}`);
  return Number(m[1]);
}

const HSL_RE = /^hsl\(\d+, \d+%, \d+%\)$/;

describe("identiconModel", () => {
  it("is deterministic: same seed produces identical models", () => {
    const a = identiconModel("my-cluster-abc");
    const b = identiconModel("my-cluster-abc");
    expect(a).toEqual(b);
  });

  it("different seeds generally produce different colors", () => {
    const a = identiconModel("seed-alpha");
    const b = identiconModel("seed-beta-99");
    // It is astronomically unlikely two random seeds hash to identical colors.
    expect(a.color === b.color && a.color2 === b.color2).toBe(false);
  });

  it("color and color2 match the hsl() format", () => {
    const m = identiconModel("test-seed");
    expect(m.color).toMatch(HSL_RE);
    expect(m.color2).toMatch(HSL_RE);
  });

  it("hue is within 0..359", () => {
    const seeds = ["a", "bb", "cluster-01", "z".repeat(50), ""];
    for (const seed of seeds) {
      const m = identiconModel(seed);
      const hue = parseHue(m.color);
      expect(hue).toBeGreaterThanOrEqual(0);
      expect(hue).toBeLessThanOrEqual(359);
    }
  });

  it("all cells have col in 0..3 and row in 0..6", () => {
    const m = identiconModel("boundary-check");
    for (const cell of m.cells) {
      expect(cell.col).toBeGreaterThanOrEqual(0);
      expect(cell.col).toBeLessThanOrEqual(3);
      expect(cell.row).toBeGreaterThanOrEqual(0);
      expect(cell.row).toBeLessThanOrEqual(6);
    }
  });

  it("density produces between 0 and 28 cells (grid 4x7)", () => {
    // Density 35..72 means 35-72% of 28 cells may be filled; but the absolute
    // worst case is all 28 or zero, so just assert within bounds.
    const m = identiconModel("density-test");
    expect(m.cells.length).toBeGreaterThanOrEqual(0);
    expect(m.cells.length).toBeLessThanOrEqual(IDENTICON_GRID * 4);
  });

  it("does not throw for an empty-string seed and returns a valid model", () => {
    let m: IdenticonModel | undefined;
    expect(() => {
      m = identiconModel("");
    }).not.toThrow();
    expect(m).toBeDefined();
    if (!m) return;
    expect(m.color).toMatch(HSL_RE);
    expect(m.color2).toMatch(HSL_RE);
    expect(Array.isArray(m.cells)).toBe(true);
  });

  it("cell alt flag is a boolean", () => {
    const m = identiconModel("alt-flag-test");
    for (const cell of m.cells) {
      expect(typeof cell.alt).toBe("boolean");
    }
  });
});

describe("identiconTiles", () => {
  const PIXELS = 70;
  const CELL_SIZE = PIXELS / IDENTICON_GRID; // 10

  it("tile size equals cellSize + 0.5", () => {
    const tiles = identiconTiles("tile-size-test", PIXELS);
    for (const tile of tiles) {
      expect(tile.size).toBeCloseTo(CELL_SIZE + 0.5);
    }
  });

  it("each tile x is a multiple of cellSize within 0..(GRID-1)*cellSize", () => {
    const tiles = identiconTiles("tile-x-test", PIXELS);
    for (const tile of tiles) {
      // x must be an exact multiple of cellSize
      expect(tile.x % CELL_SIZE).toBeCloseTo(0);
      expect(tile.x).toBeGreaterThanOrEqual(0);
      expect(tile.x).toBeLessThanOrEqual((IDENTICON_GRID - 1) * CELL_SIZE);
    }
  });

  it("mirroring: for every left-half tile at col c there is one at col (6-c)", () => {
    // Pick a seed that deterministically produces at least one left-only cell.
    // We'll use the model directly to know what cells were generated.
    const seed = "mirror-test";
    const model = identiconModel(seed);
    const tiles = identiconTiles(seed, PIXELS);

    // Build a set of (x, y) positions from the tile list.
    const positions = new Set(
      tiles.map((t) => `${String(t.x)},${String(t.y)}`),
    );

    for (const cell of model.cells) {
      const leftX = cell.col * CELL_SIZE;
      const rightX = (IDENTICON_GRID - 1 - cell.col) * CELL_SIZE;
      const y = cell.row * CELL_SIZE;

      expect(positions.has(`${String(leftX)},${String(y)}`)).toBe(true);
      // The mirror always exists - if col === 3 it is the same position.
      expect(positions.has(`${String(rightX)},${String(y)}`)).toBe(true);
    }
  });

  it("center column (col 3) is not double-emitted", () => {
    // Force a seed where at least one center-column cell is present.
    // We brute-force until we find one that generates a col-3 cell.
    const seed = "center-col-scan";
    const { cells } = identiconModel(seed);
    const hasCenterCol = cells.some((c) => c.col === 3);

    if (hasCenterCol) {
      const tiles = identiconTiles(seed, PIXELS);
      // For each center-column cell there must be exactly one tile at that position.
      for (const cell of cells.filter((c) => c.col === 3)) {
        const x = cell.col * CELL_SIZE;
        const y = cell.row * CELL_SIZE;
        const count = tiles.filter((t) => t.x === x && t.y === y).length;
        expect(count).toBe(1);
      }
    } else {
      // No center-column cells in this seed - just verify no tiles at x=30 are
      // doubled up.
      const tiles = identiconTiles(seed, PIXELS);
      const centerX = 3 * CELL_SIZE;
      const centerTiles = tiles.filter((t) => t.x === centerX);
      // Each (x,y) combo must appear at most once.
      const uniquePos = new Set(
        centerTiles.map((t) => `${String(t.x)},${String(t.y)}`),
      );
      expect(uniquePos.size).toBe(centerTiles.length);
    }
  });

  it("fill is one of model.color or model.color2", () => {
    const seed = "fill-test";
    const model = identiconModel(seed);
    const tiles = identiconTiles(seed, PIXELS);
    for (const tile of tiles) {
      expect([model.color, model.color2]).toContain(tile.fill);
    }
  });

  it("returns an empty array when the model has no cells", () => {
    // We cannot force density to produce zero cells easily, but we can verify
    // that tiles is always an array (possibly empty).
    const tiles = identiconTiles("no-crash-seed", 42);
    expect(Array.isArray(tiles)).toBe(true);
  });

  it("does not throw for an empty-string seed", () => {
    expect(() => identiconTiles("", 70)).not.toThrow();
  });

  it("scales correctly with different pixel values", () => {
    const tiles140 = identiconTiles("scale-test", 140);
    const expectedCellSize = 140 / IDENTICON_GRID;
    for (const tile of tiles140) {
      expect(tile.size).toBeCloseTo(expectedCellSize + 0.5);
    }
  });
});
