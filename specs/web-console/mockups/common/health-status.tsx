import { Icon } from "@patternfly/react-core";
import { CheckCircleIcon } from "@patternfly/react-icons";

export function HealthStatus({ label = "Healthy" }: { label?: string }) {
  return (
    <span>
      <Icon isInline status="success">
        <CheckCircleIcon aria-hidden />
      </Icon>{" "}
      {label}
    </span>
  );
}
