// Shared agent-activity cue (SDLC-AGT-17): a non-color dot paired with a text label
// for each of the three activity states, plus an optional "last seen {ago} ago"
// relative timestamp when a freshness fact is present. Color is never the only
// channel - the label is always rendered beside the dot (SDLC-AGT-13 /
// accessibility.spec). Used by both the Agent SDLC board column header and the
// per-cluster Agents table so the two surfaces communicate liveness identically.

import { Content } from "@patternfly/react-core";
import { useEffect, useState } from "react";
import type { IntlShape } from "react-intl";
import { useIntl } from "react-intl";

import type { AgentActivity } from "../../../domain/sdlc-agents";
import { messages } from "../../../messages";
import { TONE_COLOR } from "../map/colors";

const SUBTLE = "var(--pf-t--global--icon--color--subtle, #9a9da0)";

/**
 * A wall-clock timestamp that ticks every second so "last seen" and the active/
 * not-active derivation refresh live between plane refetches. An explicit override
 * (tests, the dev harness) freezes it for deterministic output and starts no timer.
 * The clock lives in the view layer; the pure derivation (`agentActivity`) is still
 * passed the value and never reads the clock itself (SDLC-AGT-06/16).
 */
export function useLiveNow(override?: number): number {
  const [now, setNow] = useState<number>(() => override ?? Date.now());
  useEffect(() => {
    if (override !== undefined) {
      return;
    }
    const id = setInterval(() => {
      setNow(Date.now());
    }, 1_000);
    return () => {
      clearInterval(id);
    };
  }, [override]);
  return override ?? now;
}

/** Dot color per state. "Not active" is the normal resting state of a cron agent, so
 *  it is muted (neutral), not an alarm; only "blind" carries a warning tone here (a
 *  real last-run failure is surfaced by the separate failure count). */
function activityDotColor(activity: AgentActivity): string {
  switch (activity) {
    case "active":
      return TONE_COLOR.success;
    case "blind":
      return TONE_COLOR.warning;
    default:
      return SUBTLE;
  }
}

function activityLabel(intl: IntlShape, activity: AgentActivity): string {
  switch (activity) {
    case "active":
      return intl.formatMessage(messages.agentActive);
    case "blind":
      return intl.formatMessage(messages.agentMonitoringBlind);
    default:
      return intl.formatMessage(messages.agentNotActive);
  }
}

/** Compact relative duration ("3h 20m", "5m"), at most two adjacent units down to
 *  minutes. The single source of truth for the board's time-in-phase and this
 *  module's "last seen" text, so the two never drift. */
export function formatDurationShort(intl: IntlShape, seconds: number): string {
  const total = Math.max(seconds, 0);
  const days = Math.floor(total / 86_400);
  const hours = Math.floor((total % 86_400) / 3_600);
  const minutes = Math.floor((total % 3_600) / 60);
  if (days > 0) {
    return intl.formatMessage(messages.sdlcAge, { days, hours });
  }
  if (hours > 0) {
    return intl.formatMessage(messages.sdlcAgeHm, { hours, minutes });
  }
  return intl.formatMessage(messages.sdlcAgeM, { minutes });
}

/** "last seen {ago} ago" when the agent has ever reported, else null. */
export function lastSeenText(
  intl: IntlShape,
  lastReportedAt: number | undefined,
  nowMs: number,
): string | null {
  if (!lastReportedAt || lastReportedAt <= 0) {
    return null;
  }
  const ago = formatDurationShort(intl, nowMs / 1000 - lastReportedAt);
  return intl.formatMessage(messages.agentLastSeen, { ago });
}

/** The "last seen {ago} ago" line for a surface, or null when the agent has never
 *  reported. Shared by the board column header and the per-cluster Agents table so
 *  liveness decay is communicated identically (SDLC-AGT-17). */
export function LastSeen({
  lastReportedAt,
  nowMs,
  className,
  style,
}: {
  readonly lastReportedAt: number | undefined;
  readonly nowMs: number;
  readonly className?: string;
  readonly style?: React.CSSProperties;
}): React.ReactElement | null {
  const intl = useIntl();
  const seen = lastSeenText(intl, lastReportedAt, nowMs);
  if (seen === null) {
    return null;
  }
  return (
    <Content component="small" className={className} style={style}>
      {seen}
    </Content>
  );
}

/** The dot + label cue for an agent's activity state. */
export function AgentActivityBadge({
  activity,
}: {
  readonly activity: AgentActivity;
}): React.ReactElement {
  const intl = useIntl();
  const label = activityLabel(intl, activity);
  return (
    <span
      style={{
        display: "inline-flex",
        alignItems: "center",
        gap: "var(--pf-t--global--spacer--xs, 4px)",
      }}
    >
      <span
        aria-hidden
        style={{
          display: "inline-block",
          width: 8,
          height: 8,
          borderRadius: "50%",
          background: activityDotColor(activity),
          flex: "0 0 auto",
        }}
      />
      <span>{label}</span>
    </span>
  );
}
