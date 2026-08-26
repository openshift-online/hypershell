import { Icon, Spinner } from "@patternfly/react-core";
import {
  BanIcon,
  CheckCircleIcon,
  ExclamationCircleIcon,
  ExclamationTriangleIcon,
  PendingIcon,
  UnknownIcon,
} from "@patternfly/react-icons";

import {
  type GatewayStatusAppearance,
  gatewayStatusAppearance,
} from "./gateway-connections";
import styles from "./gateway-status.module.css";

function GatewayStatusIcon({
  appearance,
}: {
  appearance: GatewayStatusAppearance;
}) {
  switch (appearance) {
    case "inactive":
      return (
        <Icon isInline>
          <BanIcon aria-hidden />
        </Icon>
      );
    case "success":
      return (
        <Icon isInline status="success">
          <CheckCircleIcon aria-hidden />
        </Icon>
      );
    case "warning":
      return (
        <Icon isInline status="warning">
          <ExclamationTriangleIcon aria-hidden />
        </Icon>
      );
    case "danger":
      return (
        <Icon isInline status="danger">
          <ExclamationCircleIcon aria-hidden />
        </Icon>
      );
    case "pending":
      return (
        <Icon isInline status="info">
          <PendingIcon aria-hidden />
        </Icon>
      );
    case "progress":
      return <Spinner aria-hidden isInline role="presentation" />;
    case "unknown":
      return (
        <Icon isInline>
          <UnknownIcon aria-hidden />
        </Icon>
      );
  }
}

export function GatewayStatus({
  label,
  status,
}: {
  label?: React.ReactNode;
  status: string;
}) {
  const appearance = gatewayStatusAppearance(status);

  return (
    <span
      className={
        appearance === "unknown" || appearance === "inactive"
          ? [styles.status, styles.unknown].join(" ")
          : styles.status
      }
    >
      <GatewayStatusIcon appearance={appearance} />
      <span>{label ?? status}</span>
    </span>
  );
}
