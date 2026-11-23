import {
  Alert,
  AlertActionCloseButton,
  AlertGroup,
} from "@patternfly/react-core";
import type { ReactElement, ReactNode } from "react";
import { useIntl } from "react-intl";

import { messages } from "../messages";

export function GatewayCreatedDate({
  createdAt,
}: {
  createdAt?: string;
}): ReactNode {
  const intl = useIntl();
  const createdDate = createdAt ? new Date(createdAt) : undefined;

  if (!createdDate || Number.isNaN(createdDate.getTime())) {
    return intl.formatMessage(messages.notAvailable);
  }

  return (
    <time dateTime={createdAt}>
      {intl.formatDate(createdDate, {
        day: "numeric",
        month: "short",
        year: "numeric",
      })}
    </time>
  );
}

export function GatewaySuccessAlerts({
  deletedGatewayName,
  onDismissDeleted,
  onDismissRenamed,
  renamedGatewayName,
}: {
  deletedGatewayName?: string;
  onDismissDeleted?: () => void;
  onDismissRenamed: () => void;
  renamedGatewayName?: string;
}): ReactElement | null {
  const intl = useIntl();

  if (!deletedGatewayName && !renamedGatewayName) {
    return null;
  }

  return (
    <AlertGroup
      aria-label={intl.formatMessage(messages.notifications)}
      isLiveRegion
      isToast
    >
      {renamedGatewayName ? (
        <Alert
          actionClose={
            <AlertActionCloseButton
              aria-label={intl.formatMessage(messages.close)}
              onClose={onDismissRenamed}
            />
          }
          onTimeout={onDismissRenamed}
          timeout={8000}
          title={intl.formatMessage(messages.gatewayRenamed, {
            gatewayName: renamedGatewayName,
          })}
          variant="success"
        />
      ) : null}
      {deletedGatewayName ? (
        <Alert
          actionClose={
            <AlertActionCloseButton
              aria-label={intl.formatMessage(messages.close)}
              onClose={() => {
                onDismissDeleted?.();
              }}
            />
          }
          onTimeout={() => {
            onDismissDeleted?.();
          }}
          timeout={8000}
          title={intl.formatMessage(messages.gatewayDeleted, {
            gatewayName: deletedGatewayName,
          })}
          variant="success"
        />
      ) : null}
    </AlertGroup>
  );
}
