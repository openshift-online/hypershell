// Shared rendering of a release bundle's timestamp. The server sends an RFC3339
// Zulu/UTC instant (ReleaseBundle.date); users want to read it three ways at once:
//   - a relative string ("3 hr. ago") for a quick sense of recency,
//   - the absolute instant in their OWN browser timezone (not UTC) for the real time,
//   - and the original Zulu/UTC instant, on hover, as the unambiguous source value.
// Both the freight cards and the bundle detail sidebar use this, so the formatting
// lives here once rather than being duplicated (and drifting) in two places.

import { Tooltip } from "@patternfly/react-core";
import { useIntl, type IntlShape } from "react-intl";

export interface ReleaseTimeProps {
  /** RFC3339/ISO instant from the server (ReleaseBundle.date). */
  readonly iso: string | null;
  /**
   * "full" (sidebar): local absolute time followed by the relative string.
   * "relative" (compact freight card): just the relative string.
   * Both carry the local-full + Zulu/UTC instant in the hover tooltip.
   */
  readonly mode?: "full" | "relative";
  /** Optional class for the visible text (e.g. the muted freight-card line). */
  readonly className?: string;
  /** Optional inline style for the visible text. */
  readonly style?: React.CSSProperties;
}

type RelUnit = Intl.RelativeTimeFormatUnit;

// Pick the coarsest sensible unit for a relative time, so a 90-minute-old release
// reads "2 hr. ago" rather than "90 min. ago". Returns a NEGATIVE value for the past
// (the common case), which Intl renders as "... ago". Exported for unit testing.
export function selectRelativeUnit(
  fromMs: number,
  nowMs: number,
): { value: number; unit: RelUnit } {
  // Round by magnitude then reapply sign, so a 1.5-unit span rounds AWAY from zero
  // (90 min -> 2 hr) rather than toward +inf the way Math.round(-1.5) === -1 would.
  const round = (x: number): number => Math.sign(x) * Math.round(Math.abs(x));
  const sec = round((fromMs - nowMs) / 1000);
  const abs = Math.abs(sec);
  if (abs < 60) return { value: sec, unit: "second" };
  const min = round(sec / 60);
  if (Math.abs(min) < 60) return { value: min, unit: "minute" };
  const hr = round(sec / 3600);
  if (Math.abs(hr) < 24) return { value: hr, unit: "hour" };
  const day = round(sec / 86400);
  if (Math.abs(day) < 30) return { value: day, unit: "day" };
  const month = round(day / 30);
  if (Math.abs(month) < 12) return { value: month, unit: "month" };
  const year = round(day / 365);
  return { value: year, unit: "year" };
}

interface Parts {
  relative: string;
  localShort: string;
  localFull: string;
  zulu: string;
}

// Build all three representations from an ISO instant, or null if it can't be parsed.
// Exported so both the component and its tests share one source of truth.
export function releaseTimeParts(
  intl: IntlShape,
  iso: string,
  nowMs: number = Date.now(),
): Parts | null {
  const d = new Date(iso);
  const ms = d.getTime();
  if (Number.isNaN(ms)) return null;
  const { value, unit } = selectRelativeUnit(ms, nowMs);
  return {
    relative: intl.formatRelativeTime(value, unit, {
      numeric: "auto",
      style: "short",
    }),
    localShort: intl.formatDate(d, { dateStyle: "medium", timeStyle: "short" }),
    // dateStyle:"full"/timeStyle:"long" includes the browser's timezone name, making
    // it clear the absolute time is LOCAL (vs the UTC line below it).
    localFull: intl.formatDate(d, { dateStyle: "full", timeStyle: "long" }),
    zulu: d.toISOString(),
  };
}

/**
 * Renders a release instant as relative + browser-local time, with the local-full and
 * Zulu/UTC instants in a tooltip. Renders nothing when `iso` is null; falls back to the
 * raw string (no tooltip) if it can't be parsed, so a bad value is never hidden.
 */
export function ReleaseTime({
  iso,
  mode = "full",
  className,
  style,
}: ReleaseTimeProps): React.ReactElement | null {
  const intl = useIntl();
  if (!iso) return null;
  const parts = releaseTimeParts(intl, iso);
  if (!parts) {
    return (
      <span className={className} style={style}>
        {iso}
      </span>
    );
  }
  const visible =
    mode === "full"
      ? `${parts.localShort} (${parts.relative})`
      : parts.relative;
  const tooltip = `${parts.localFull}\nUTC: ${parts.zulu}`;
  return (
    <Tooltip
      content={<span style={{ whiteSpace: "pre-line" }}>{tooltip}</span>}
    >
      <span className={className} style={style}>
        {visible}
      </span>
    </Tooltip>
  );
}
