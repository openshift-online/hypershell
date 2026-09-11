import { ProgressStep, ProgressStepper } from "@patternfly/react-core";
import { useIntl } from "react-intl";

import type {
  ProvisioningCondition,
  ProvisioningConditionStatus,
} from "../application/gateway-types";
import { messages } from "../messages";

type ProgressStepVariant =
  "danger" | "default" | "pending" | "success" | "warning";

const conditionLabels: Record<string, keyof typeof messages> = {
  DatabaseReady: "provisioningStepProvisioningDatabase",
  EnvironmentReady: "provisioningStepPreparingEnvironment",
  GatewayDeployed: "provisioningStepDeployingGateway",
  GatewayHealthy: "provisioningStepVerifyingHealth",
  IdentityProviderReady: "provisioningStepConfiguringIdentityProvider",
};

function stepVariant(
  conditionStatus: ProvisioningConditionStatus,
  conditionType: string,
  phase: string | undefined,
): ProgressStepVariant {
  switch (conditionStatus) {
    case "Pending":
      return "default";
    case "InProgress":
      return "pending";
    case "Complete":
      return "success";
    case "Failed":
      if (
        conditionType === "GatewayHealthy" &&
        phase?.toLocaleLowerCase() === "degraded"
      ) {
        return "warning";
      }
      return "danger";
    default:
      return "default";
  }
}

interface GatewayProvisioningStepperProps {
  conditions: readonly ProvisioningCondition[];
  phase?: string;
}

export function GatewayProvisioningStepper({
  conditions,
  phase,
}: GatewayProvisioningStepperProps) {
  const intl = useIntl();

  if (conditions.length === 0) {
    return null;
  }

  return (
    <ProgressStepper
      aria-label={intl.formatMessage(messages.provisioningProgressLabel)}
    >
      {conditions.map((condition) => {
        const messageKey = conditionLabels[condition.type];
        const title = messageKey
          ? intl.formatMessage(messages[messageKey])
          : condition.type;
        const variant = stepVariant(
          condition.conditionStatus,
          condition.type,
          phase,
        );
        const description =
          condition.conditionStatus === "Failed" && condition.message
            ? condition.message
            : undefined;

        return (
          <ProgressStep
            description={description}
            id={condition.type}
            isCurrent={condition.conditionStatus === "InProgress"}
            key={condition.type}
            titleId={`${condition.type}-title`}
            variant={variant}
          >
            {title}
          </ProgressStep>
        );
      })}
    </ProgressStepper>
  );
}
