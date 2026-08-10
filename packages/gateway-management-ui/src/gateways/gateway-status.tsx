import { Icon, Spinner } from "@patternfly/react-core";
import {
  CheckCircleIcon,
  ExclamationCircleIcon,
  ExclamationTriangleIcon,
  PendingIcon,
  UnknownIcon,
} from "@patternfly/react-icons";

import { gatewayStatusAppearance } from "./gateway-connections";
import styles from "./gateway-status.module.css";

function GatewayStatusIcon({ status }: { status: string }) {
  switch (gatewayStatusAppearance(status)) {
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

export function GatewayStatus({ status }: { status: string }) {
  return (
    <span className={styles.status}>
      <GatewayStatusIcon status={status} />
      <span>{status}</span>
    </span>
  );
}
