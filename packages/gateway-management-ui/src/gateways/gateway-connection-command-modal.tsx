import {
  Content,
  Stack,
  StackItem,
  Button,
  Modal,
  ModalBody,
  ModalFooter,
  ModalHeader,
  ModalVariant,
} from "@patternfly/react-core";
import { useIntl } from "react-intl";

import {
  buildGatewayAddCommand,
  type GatewayConnection,
} from "./gateway-connections";
import { CommandBlock } from "./editable-command";
import { messages } from "../messages";
import styles from "./gateway-connection-steps.module.css";

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

  const connectionCommand = buildGatewayAddCommand(gateway);

  return (
    <Modal isOpen={isOpen} onClose={onClose} variant={ModalVariant.medium}>
      <ModalHeader
        title={intl.formatMessage(messages.connectionCommandModal, {
          gatewayName: gateway.name,
        })}
      />
      <ModalBody>
        <Stack hasGutter>
          <StackItem>
            <Content component="p">
              {intl.formatMessage(messages.connectionCommandHelper)}
            </Content>
          </StackItem>
          <StackItem>
            {connectionCommand ? (
              <div className={styles.steps}>
                <CommandBlock
                  copyAriaLabel={intl.formatMessage(
                    messages.copyConnectionCommand,
                    {
                      gatewayName: gateway.name,
                    },
                  )}
                  copyText={connectionCommand}
                />
              </div>
            ) : (
              <p>{intl.formatMessage(messages.connectionLoginUnavailable)}</p>
            )}
          </StackItem>
        </Stack>
      </ModalBody>
      <ModalFooter>
        <Button onClick={onClose} variant="primary">
          {intl.formatMessage(messages.close)}
        </Button>
      </ModalFooter>
    </Modal>
  );
}
