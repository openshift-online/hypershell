// Renders a domain StatusBadge as a PatternFly Label. Tone maps to a semantic
// status token (never a raw color); the label key resolves to translated copy so
// state is carried by BOTH color and text (brand-color spec UI-BRAND-03).

import { Label, type LabelProps } from "@patternfly/react-core";
import type { MessageDescriptor } from "react-intl";
import { useIntl } from "react-intl";

import type { SemanticTone, StatusBadge } from "../../domain/status";
import { messages } from "../../messages";

const TONE_STATUS: Record<SemanticTone, LabelProps["status"] | undefined> = {
  success: "success",
  warning: "warning",
  danger: "danger",
  info: "info",
  unknown: undefined, // grey default - never colored as success
};

const LABEL_MESSAGES: Record<string, MessageDescriptor> = {
  passed: messages.statusPassed,
  failed: messages.statusFailed,
  pending: messages.statusPending,
  healthy: messages.statusHealthy,
  progressing: messages.statusProgressing,
  degraded: messages.statusDegraded,
  suspended: messages.statusSuspended,
  synced: messages.statusSynced,
  outOfSync: messages.statusOutOfSync,
  unknown: messages.statusUnknown,
};

export function StatusLabel({
  badge,
}: {
  badge: StatusBadge;
}): React.ReactElement {
  const intl = useIntl();
  const descriptor = LABEL_MESSAGES[badge.labelKey] ?? messages.statusUnknown;
  return (
    <Label status={TONE_STATUS[badge.tone]} variant="outline" isCompact>
      {intl.formatMessage(descriptor)}
    </Label>
  );
}
