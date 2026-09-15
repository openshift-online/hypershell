import { ProgressStep, ProgressStepper } from "@patternfly/react-core";
import { useIntl } from "react-intl";

import type {
  ProvisioningCondition,
  ProvisioningConditionStatus,
} from "../application/gateway-types";
import { messages } from "../messages";

type ProgressStepVariant =
  "danger" | "default" | "info" | "pending" | "success" | "warning";

const activeLabels: Record<string, keyof typeof messages> = {
  ConsoleReady: "provisioningStepStartingConsole",
  DatabaseReady: "provisioningStepProvisioningDatabase",
  EnvironmentReady: "provisioningStepPreparingEnvironment",
  GatewayDeployed: "provisioningStepDeployingGateway",
  GatewayHealthy: "provisioningStepVerifyingHealth",
  IdentityProviderReady: "provisioningStepConfiguringIdentityProvider",
  Provisioned: "provisioningStepProvisioned",
};

const completedLabels: Record<string, keyof typeof messages> = {
  ConsoleReady: "provisioningStepConsoleReady",
  DatabaseReady: "provisioningStepDatabaseReady",
  EnvironmentReady: "provisioningStepEnvironmentReady",
  GatewayDeployed: "provisioningStepGatewayDeployed",
  GatewayHealthy: "provisioningStepHealthVerified",
  IdentityProviderReady: "provisioningStepIdentityProviderReady",
  Provisioned: "provisioningStepProvisioned",
};

const defaultConditionOrder: readonly string[] = [
  "EnvironmentReady",
  "DatabaseReady",
  "IdentityProviderReady",
  "GatewayDeployed",
  "GatewayHealthy",
  "ConsoleReady",
  "Provisioned",
];

function stepVariant(
  conditionStatus: ProvisioningConditionStatus,
  conditionType: string,
  phase: string | undefined,
): ProgressStepVariant {
  switch (conditionStatus) {
    case "InProgress":
      return "info";
    case "Pending":
      return "default";
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

function consoleConditionStatus(
  consoleReady: boolean,
  serverConditions: readonly ProvisioningCondition[],
): ProvisioningConditionStatus {
  if (consoleReady) {
    return "Complete";
  }
  const allServerComplete =
    serverConditions.length > 0 &&
    serverConditions.every((c) => c.conditionStatus === "Complete");
  if (allServerComplete) {
    return "InProgress";
  }
  return "Pending";
}

interface GatewayProvisioningStepperProps {
  conditions: readonly ProvisioningCondition[];
  consoleReady?: boolean;
  phase?: string;
}

export function GatewayProvisioningStepper({
  conditions,
  consoleReady = false,
  phase,
}: GatewayProvisioningStepperProps) {
  const intl = useIntl();

  const consoleCondition: ProvisioningCondition = {
    conditionStatus: consoleConditionStatus(consoleReady, conditions),
    message: "",
    type: "ConsoleReady",
  };

  const allDone =
    conditions.length > 0 &&
    conditions.every((c) => c.conditionStatus === "Complete") &&
    consoleReady;

  const provisionedCondition: ProvisioningCondition = {
    conditionStatus: allDone ? "Complete" : "Pending",
    message: "",
    type: "Provisioned",
  };

  const effectiveConditions: readonly ProvisioningCondition[] =
    conditions.length > 0
      ? [...conditions, consoleCondition, provisionedCondition]
      : defaultConditionOrder.map((type, index) => ({
          conditionStatus: index === 0 ? "InProgress" : "Pending",
          message: "",
          type,
        }));

  return (
    <ProgressStepper
      aria-label={intl.formatMessage(messages.provisioningProgressLabel)}
    >
      {effectiveConditions.map((condition) => {
        const isComplete = condition.conditionStatus === "Complete";
        const labels = isComplete ? completedLabels : activeLabels;
        const messageKey = labels[condition.type];
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
