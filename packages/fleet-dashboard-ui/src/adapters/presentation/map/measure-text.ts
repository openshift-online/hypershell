// Text measurement for the SVG map card (FLT-CAP-04). SVG has no synchronous
// glyph-width API, so a <rect> cannot be sized to its text without measuring. We
// measure through a cached offscreen 2D canvas and fall back to a per-character
// estimate where no canvas exists (server-side rendering, jsdom tests) - the
// fallback deliberately slightly OVER-estimates so a chip never clips its text.
// This lives in the presentation layer, not in the pure map layout, so the
// layout geometry stays DOM-free (FLT-MAP-06).

/** Shared offscreen canvas context, created lazily and reused across calls. */
let ctx: CanvasRenderingContext2D | null | undefined;

function context(): CanvasRenderingContext2D | null {
  if (ctx !== undefined) {
    return ctx;
  }
  // document may be absent (SSR) and getContext may return null (jsdom); either
  // way we remember null and use the fallback from then on.
  if (typeof document === "undefined") {
    ctx = null;
    return ctx;
  }
  const canvas = document.createElement("canvas");
  ctx = canvas.getContext("2d");
  return ctx;
}

/** Memoised measurements keyed by `${weight}:${size}:${text}`. */
const cache = new Map<string, number>();

/** Per-em width estimate for the canvas-less fallback; heavier weights are wider.
 *  Chosen to slightly over-estimate so a chip sized from it never clips. */
function fallbackWidth(
  text: string,
  fontSize: number,
  fontWeight: number,
): number {
  const perEm = fontWeight >= 600 ? 0.62 : 0.56;
  return text.length * fontSize * perEm;
}

/**
 * The rendered width, in SVG user units, of `text` at the given font size/weight.
 * Uses the cached canvas when available, else a slight over-estimate. Memoised per
 * (weight, size, text).
 */
export function measureText(
  text: string,
  fontSize: number,
  fontWeight = 400,
): number {
  const key = `${String(fontWeight)}:${String(fontSize)}:${text}`;
  const hit = cache.get(key);
  if (hit !== undefined) {
    return hit;
  }
  const c = context();
  let width: number;
  if (c) {
    c.font = `${String(fontWeight)} ${String(fontSize)}px sans-serif`;
    width = c.measureText(text).width;
  } else {
    width = fallbackWidth(text, fontSize, fontWeight);
  }
  cache.set(key, width);
  return width;
}

/**
 * Shorten `text` with a trailing "..." so it fits within `maxWidth` at the given
 * font. Returns the text unchanged when it already fits. Binary-searches the
 * largest prefix whose `"<prefix>..."` still fits; returns a bare "..." when even
 * one character does not fit.
 */
export function truncateToWidth(
  text: string,
  fontSize: number,
  fontWeight: number,
  maxWidth: number,
): string {
  if (measureText(text, fontSize, fontWeight) <= maxWidth) {
    return text;
  }
  const ellipsis = "...";
  // Largest prefix length whose prefix + ellipsis fits. Binary search over [0, len).
  let lo = 0;
  let hi = text.length;
  while (lo < hi) {
    const mid = Math.ceil((lo + hi) / 2);
    const candidate = text.slice(0, mid) + ellipsis;
    if (measureText(candidate, fontSize, fontWeight) <= maxWidth) {
      lo = mid;
    } else {
      hi = mid - 1;
    }
  }
  return lo > 0 ? text.slice(0, lo) + ellipsis : ellipsis;
}
