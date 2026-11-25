// Shared formatting for the Delivery tab (fleet-dashboard-delivery-pipeline
// .spec): durations, clock times, gate names, and stage state icons.

import { Icon } from "@patternfly/react-core";
import {
  CheckCircleIcon,
  ExclamationCircleIcon,
  InProgressIcon,
  PendingIcon,
  QuestionCircleIcon,
} from "@patternfly/react-icons";
import type { IntlShape, MessageDescriptor } from "react-intl";

import type {
  DeliverySourceKey,
  DeliveryStageState,
} from "../../../domain/delivery";
import { deliveryMessages as m } from "./delivery-messages";

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

export function formatClock(
  intl: IntlShape,
  iso: string | number | null,
): string {
  if (iso === null) {
    return "";
  }
  const ms = typeof iso === "number" ? iso : Date.parse(iso);
  if (Number.isNaN(ms)) {
    return "";
  }
  return intl.formatTime(ms, {
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
  });
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
