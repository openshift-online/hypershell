// A "plane" is one independently-refreshed data source. Every plane carries its
// own freshness + error state so the UI can degrade one region without blanking
// the others (per the data-architecture spec's per-source degradation).

export interface Plane<T> {
  readonly data: T;
  /** RFC3339 timestamp the server generated this payload. */
  readonly generatedAt: string;
  /** True when the server is serving a last-good value because a refresh failed. */
  readonly stale: boolean;
  /** Human-readable error for the source, when the last refresh failed. */
  readonly error: string | null;
}

/** Freshness verdict for a plane, relative to a reference instant (ms epoch). */
export type Freshness = "fresh" | "aging" | "stale";

export function planeFreshness(
  plane: Pick<Plane<unknown>, "generatedAt" | "stale">,
  nowMs: number,
  agingAfterMs: number,
  staleAfterMs: number,
): Freshness {
  if (plane.stale) {
    return "stale";
  }
  const ageMs = nowMs - Date.parse(plane.generatedAt);
  if (Number.isNaN(ageMs)) {
    return "stale";
  }
  if (ageMs >= staleAfterMs) {
    return "stale";
  }
  if (ageMs >= agingAfterMs) {
    return "aging";
  }
  return "fresh";
}
