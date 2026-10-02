// Tiny SVG geometry string helpers. The lint config forbids interpolating raw
// numbers into template literals (@typescript-eslint/restrict-template-expressions),
// so every coordinate goes through `f`, which also guards against NaN/Infinity
// leaking into the DOM as invalid attribute values.

/** Format a number as a fixed-2 string safe for an SVG attribute. */
export function f(value: number): string {
  return Number.isFinite(value) ? value.toFixed(2) : "0";
}

/** A single "x,y" point for points/polyline/polygon. */
export function pt(x: number, y: number): string {
  return `${f(x)},${f(y)}`;
}

/** A "x y w h" viewBox string. */
export function viewBox(x: number, y: number, w: number, h: number): string {
  return `${f(x)} ${f(y)} ${f(w)} ${f(h)}`;
}
