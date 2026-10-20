import { Icon } from "@patternfly/react-core";
import {
  CheckCircleIcon,
  ExclamationCircleIcon,
  ExclamationTriangleIcon,
} from "@patternfly/react-icons";

export type HealthStatusAppearance = "good" | "warning" | "danger";

const statusPresentation = {
  danger: {
    icon: ExclamationCircleIcon,
    patternFlyStatus: "danger" as const,
  },
  warning: {
    icon: ExclamationTriangleIcon,
    patternFlyStatus: "warning" as const,
  },
  good: {
    icon: CheckCircleIcon,
    patternFlyStatus: "success" as const,
  },
};

export function HealthStatus({
  appearance = "good",
  label = "Healthy",
}: {
  appearance?: HealthStatusAppearance;
  label?: string;
}) {
  const { icon: StatusIcon, patternFlyStatus } = statusPresentation[appearance];

  return (
    <span>
      <Icon isInline status={patternFlyStatus}>
        <StatusIcon aria-hidden />
      </Icon>{" "}
      {label}
    </span>
  );
}
