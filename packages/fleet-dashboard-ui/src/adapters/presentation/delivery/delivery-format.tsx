// Shared formatting for the Delivery tab (fleet-dashboard-delivery-pipeline
// .spec): durations, clock times with their full date on hover, bundle names
// with their identicon, gate names, and stage state icons.

import { Icon, Tooltip } from "@patternfly/react-core";
import {
  CheckCircleIcon,
  ExclamationCircleIcon,
  InProgressIcon,
  PendingIcon,
  QuestionCircleIcon,
} from "@patternfly/react-icons";
import { useRef } from "react";
import { useIntl, type IntlShape, type MessageDescriptor } from "react-intl";

import type {
  DeliverySourceKey,
  DeliveryStageState,
} from "../../../domain/delivery";
import { identiName } from "../../../domain/map/identiname";
import { BundleIcon } from "../shared/bundle-icon";
import { deliveryMessages as m } from "./delivery-messages";
import styles from "./delivery-view.module.css";

export const STATE_LABEL: Readonly<
  Record<DeliveryStageState, MessageDescriptor>
> = {
  "not-started": m.stateNotStarted,
  waiting: m.stateWaiting,
  running: m.stateRunning,
  passed: m.statePassed,
  failed: m.stateFailed,
  "not-needed": m.stateNotNeeded,
  unknown: m.stateUnknown,
};

export function formatDuration(intl: IntlShape, ms: number): string {
  const minutes = Math.floor(ms / 60_000);
  if (minutes < 1) {
    return intl.formatMessage(m.durationLessThanMinute);
  }
  if (minutes < 60) {
    return intl.formatMessage(m.durationMinutes, { minutes });
  }
  const hours = Math.floor(minutes / 60);
  if (hours < 48) {
    return intl.formatMessage(m.durationHours, {
      hours,
      minutes: minutes % 60,
    });
  }
  return intl.formatMessage(m.durationDays, {
    days: Math.floor(hours / 24),
    hours: hours % 24,
  });
}

function toMs(iso: string | number | null): number | null {
  if (iso === null) {
    return null;
  }
  const ms = typeof iso === "number" ? iso : Date.parse(iso);
  return Number.isNaN(ms) ? null : ms;
}

function sameDay(a: number, b: number): boolean {
  return new Date(a).toDateString() === new Date(b).toDateString();
}

/**
 * A 24-hour clock time. With nowMs, a time on another day than nowMs also
 * names its date (FLT-DLV-24).
 */
export function formatClock(
  intl: IntlShape,
  iso: string | number | null,
  nowMs?: number,
): string {
  const ms = toMs(iso);
  if (ms === null) {
    return "";
  }
  const clock = intl.formatTime(ms, {
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
  });
  if (nowMs === undefined || sameDay(ms, nowMs)) {
    return clock;
  }
  const date = intl.formatDate(ms, {
    weekday: "short",
    day: "numeric",
    month: "short",
  });
  return `${date} ${clock}`;
}

/** The full date and time, with seconds and time zone (FLT-DLV-24). */
export function formatFullTime(intl: IntlShape, ms: number): string {
  return intl.formatDate(ms, {
    weekday: "short",
    day: "numeric",
    month: "short",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hourCycle: "h23",
    timeZoneName: "short",
  });
}

/**
 * A shown time that reveals its full date and time on hover and keyboard
 * focus (FLT-DLV-24). The tooltip uses a trigger ref, so the time stays a
 * phrasing element inside a paragraph.
 */
export function FullTime({
  ms,
  children,
}: {
  readonly ms: number;
  readonly children: React.ReactNode;
}) {
  const intl = useIntl();
  const ref = useRef<HTMLTimeElement>(null);
  return (
    <>
      {/* eslint-disable jsx-a11y/no-noninteractive-tabindex -- Keyboard focus reveals the full date and time (FLT-DLV-24). */}
      <time
        className={styles.time}
        dateTime={new Date(ms).toISOString()}
        ref={ref}
        tabIndex={0}
      >
        {children}
      </time>
      {/* eslint-enable jsx-a11y/no-noninteractive-tabindex */}
      <Tooltip content={formatFullTime(intl, ms)} triggerRef={ref} />
    </>
  );
}

function escapeRegExp(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

/**
 * Text that names clock times and bundles: each time the text shows, among
 * times, becomes a FullTime, and each bundle alias, among bundles (digests),
 * gets its identicon (FLT-DLV-24). nowMs must be the one the text was
 * formatted with.
 */
export function RichText({
  text,
  times = [],
  bundles = [],
  nowMs,
}: {
  readonly text: string;
  readonly times?: readonly (string | number | null)[];
  readonly bundles?: readonly (string | null)[];
  readonly nowMs?: number;
}) {
  const intl = useIntl();
  const clocks = new Map<string, number>();
  for (const t of times) {
    const ms = toMs(t);
    const label = formatClock(intl, t, nowMs);
    if (ms !== null && label !== "" && !clocks.has(label)) {
      clocks.set(label, ms);
    }
  }
  const aliases = new Map<string, string>();
  for (const d of bundles) {
    if (d) {
      aliases.set(identiName(d), d);
    }
  }
  const labels = [...clocks.keys(), ...aliases.keys()];
  if (labels.length === 0) {
    return <>{text}</>;
  }
  const pattern = new RegExp(
    labels
      .sort((a, b) => b.length - a.length)
      .map(escapeRegExp)
      .join("|"),
    "g",
  );
  const parts: React.ReactNode[] = [];
  let last = 0;
  for (const match of text.matchAll(pattern)) {
    const at = match.index;
    const label = match[0];
    if (at > last) {
      parts.push(text.slice(last, at));
    }
    const digest = aliases.get(label);
    parts.push(
      digest !== undefined ? (
        <BundleName digest={digest} key={at} />
      ) : (
        <FullTime key={at} ms={clocks.get(label) ?? 0}>
          {label}
        </FullTime>
      ),
    );
    last = at + label.length;
  }
  if (last < text.length) {
    parts.push(text.slice(last));
  }
  return <>{parts}</>;
}

/**
 * A bundle's name: its Initials identicon and alias, seeded by the digest, as on
 * every Glass tab (FLT-DLV-24). Without a digest, the fallback alone.
 */
export function BundleName({
  digest,
  fallback = null,
}: {
  readonly digest: string | null;
  readonly fallback?: string | null;
}) {
  if (!digest) {
    return <>{fallback ?? ""}</>;
  }
  return (
    <span className="pf-v6-u-text-nowrap">
      <BundleIcon seed={digest} />
      {identiName(digest)}
    </span>
  );
}

export function gateName(intl: IntlShape, gate: string | null): string {
  if (gate === null) {
    return "";
  }
  if (gate.includes("analysis")) {
    return intl.formatMessage(m.gateAnalysis);
  }
  if (gate.includes("health")) {
    return intl.formatMessage(m.gateHealth);
  }
  return gate;
}

export function StageStateIcon({
  state,
}: {
  readonly state: DeliveryStageState;
}) {
  switch (state) {
    case "running":
      return (
        <Icon isInline status="info">
          <InProgressIcon aria-hidden />
        </Icon>
      );
    case "passed":
      return (
        <Icon isInline status="success">
          <CheckCircleIcon aria-hidden />
        </Icon>
      );
    case "failed":
      return (
        <Icon isInline status="danger">
          <ExclamationCircleIcon aria-hidden />
        </Icon>
      );
    case "waiting":
      return (
        <Icon isInline>
          <PendingIcon aria-hidden />
        </Icon>
      );
    case "unknown":
      return (
        <Icon isInline>
          <QuestionCircleIcon aria-hidden />
        </Icon>
      );
    default:
      return null;
  }
}

export const SOURCE_LABEL: Readonly<
  Record<DeliverySourceKey, MessageDescriptor>
> = {
  upstream: m.sourceUpstream,
  registry: m.sourceRegistry,
  gitops: m.sourceGitops,
  promotion: m.sourcePromotion,
  history: m.sourceHistory,
};
