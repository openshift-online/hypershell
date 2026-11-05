import { Button, Modal, ModalVariant } from "@patternfly/react-core";
import { useIntl } from "react-intl";

import { type GatewayConnection, shellArgument } from "./gateway-connections";
import { CommandBlock } from "./editable-command";
import { messages } from "../messages";

export interface GatewayConnectionCommandModalProps {
  gateway: GatewayConnection;
  isOpen: boolean;
  onClose: () => void;
}

export function GatewayConnectionCommandModal({
  gateway,
  isOpen,
  onClose,
}: GatewayConnectionCommandModalProps) {
  const intl = useIntl();

  // Build the CLI connection command
  const connectionCommand = gateway.endpoint
    ? `openshell gateway add ${shellArgument(gateway.name)} ${shellArgument(gateway.endpoint)}`
    : "";

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={intl.formatMessage(messages.connectionCommandModal, {
        gatewayName: gateway.name,
      })}
      variant={ModalVariant.medium}
      actions={[
        <Button key="close" onClick={onClose} variant="primary">
          {intl.formatMessage(messages.close)}
        </Button>,
      ]}
    >
      {connectionCommand ? (
        <CommandBlock
          copyAriaLabel={intl.formatMessage(messages.copyConnectionCommand, {
            gatewayName: gateway.name,
          })}
          copyText={connectionCommand}
        />
      ) : (
        <p>{intl.formatMessage(messages.connectionLoginUnavailable)}</p>
      )}
    </Modal>
  );
}
