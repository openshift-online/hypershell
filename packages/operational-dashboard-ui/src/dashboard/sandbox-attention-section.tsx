import { Label, LabelGroup, Title } from "@patternfly/react-core";
import RhUiAsleepIcon from "@patternfly/react-icons/dist/esm/icons/rh-ui-asleep-icon";
import RhUiClockIcon from "@patternfly/react-icons/dist/esm/icons/rh-ui-clock-icon";
import RhUiDisconnectedIcon from "@patternfly/react-icons/dist/esm/icons/rh-ui-disconnected-icon";
import { FormattedMessage, useIntl } from "react-intl";

import type { OperationalMetric } from "../application/dashboard-types";
import { messages } from "../messages";

interface AttentionLabel {
  count: number;
  icon: typeof RhUiClockIcon;
  labelMessage: (typeof messages)["sandboxAttentionExpiringSoon"];
  status: "danger" | "warning" | "info";
}

function attentionLabels(metric: OperationalMetric): AttentionLabel[] {
  const labels: AttentionLabel[] = [];
  if (metric.expiringSandboxes !== undefined && metric.expiringSandboxes > 0) {
    labels.push({
      count: metric.expiringSandboxes,
      icon: RhUiClockIcon,
      labelMessage: messages.sandboxAttentionExpiringSoon,
      status: "warning",
    });
  }
  if (metric.orphanedSandboxes !== undefined && metric.orphanedSandboxes > 0) {
    labels.push({
      count: metric.orphanedSandboxes,
      icon: RhUiDisconnectedIcon,
      labelMessage: messages.sandboxAttentionOrphaned,
      status: "danger",
    });
  }
  if (metric.idleSandboxes !== undefined && metric.idleSandboxes > 0) {
    labels.push({
      count: metric.idleSandboxes,
      icon: RhUiAsleepIcon,
      labelMessage: messages.sandboxAttentionIdle,
      status: "info",
    });
  }
  return labels;
}

export function hasSandboxAttentionData(metric: OperationalMetric): boolean {
  return (
    metric.orphanedSandboxes !== undefined ||
    metric.expiringSandboxes !== undefined ||
    metric.idleSandboxes !== undefined
  );
}

/** True when at least one attention count is greater than zero. */
export function hasSandboxAttentionRequired(
  metric: OperationalMetric,
): boolean {
  return attentionLabels(metric).length > 0;
}

export function SandboxAttentionSection({
  metric,
}: Readonly<{ metric: OperationalMetric }>) {
  const intl = useIntl();
  const labels = attentionLabels(metric);

  if (labels.length === 0) {
    if (!hasSandboxAttentionData(metric)) {
      return (
        <div className="hypershell-dashboard-sandbox-attention">
          <Title headingLevel="h3" size="md">
            <FormattedMessage {...messages.sandboxAttentionRequired} />
          </Title>
          <p className="hypershell-dashboard-sandbox-attention__unavailable">
            <FormattedMessage {...messages.sandboxAttentionUnavailable} />
          </p>
        </div>
      );
    }
    return null;
  }

  return (
    <div className="hypershell-dashboard-sandbox-attention">
      <Title headingLevel="h3" size="md">
        <FormattedMessage {...messages.sandboxAttentionRequired} />
      </Title>
      <LabelGroup
        aria-label={intl.formatMessage(messages.sandboxAttentionRequired)}
        numLabels={labels.length}
      >
        {labels.map((attentionLabel) => {
          const Icon = attentionLabel.icon;
          const text = intl.formatMessage(attentionLabel.labelMessage, {
            count: attentionLabel.count,
          });
          return (
            <Label
              icon={<Icon />}
              key={attentionLabel.labelMessage.id}
              status={attentionLabel.status}
              variant="outline"
            >
              {text}
            </Label>
          );
        })}
      </LabelGroup>
    </div>
  );
}
