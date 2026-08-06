import {
  Alert,
  Button,
  Modal,
  ModalBody,
  ModalFooter,
  ModalHeader,
  ModalVariant,
  Stack,
  StackItem,
} from "@patternfly/react-core";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId } from "react";
import { FormattedMessage, useIntl } from "react-intl";

import { messages } from "../../i18n/messages";
import { deleteGateway, gatewayQueryKey } from "./gateway-data";

interface GatewayDeleteDialogProps {
  gatewayId: string;
  gatewayName: string;
  isOpen: boolean;
  onClose: () => void;
  onDeleted: () => void;
}

export function GatewayDeleteDialog({
  gatewayId,
  gatewayName,
  isOpen,
  onClose,
  onDeleted,
}: GatewayDeleteDialogProps) {
  const intl = useIntl();
  const queryClient = useQueryClient();
  const descriptionId = useId();
  const titleId = useId();
  const deletion = useMutation({
    mutationFn: () => deleteGateway(gatewayId),
    onSuccess: async () => {
      queryClient.removeQueries({
        exact: true,
        queryKey: gatewayQueryKey(gatewayId),
      });
      onDeleted();
      await queryClient.invalidateQueries({
        exact: true,
        queryKey: ["gateways"],
      });
    },
  });

  const close = () => {
    deletion.reset();
    onClose();
  };

  return (
    <Modal
      aria-describedby={descriptionId}
      aria-labelledby={titleId}
      isOpen={isOpen}
      onClose={deletion.isPending ? undefined : close}
      variant={ModalVariant.small}
    >
      <ModalHeader
        labelId={titleId}
        title={intl.formatMessage(messages.deleteGatewayTitle, {
          gatewayName,
        })}
      />
      <ModalBody>
        <Stack hasGutter>
          <StackItem id={descriptionId}>
            <FormattedMessage
              {...messages.deleteGatewayConfirmation}
              values={{ gatewayName }}
            />
          </StackItem>
          {deletion.isError ? (
            <StackItem>
              <Alert
                isInline
                title={intl.formatMessage(messages.gatewayDeleteError)}
                variant="danger"
              >
                <FormattedMessage {...messages.gatewayDeleteErrorBody} />
              </Alert>
            </StackItem>
          ) : null}
        </Stack>
      </ModalBody>
      <ModalFooter>
        <Button
          isDisabled={deletion.isPending}
          isLoading={deletion.isPending}
          onClick={() => {
            deletion.mutate();
          }}
          spinnerAriaValueText={intl.formatMessage(messages.deletingGateway)}
          variant="danger"
        >
          <FormattedMessage {...messages.deleteGateway} />
        </Button>
        <Button isDisabled={deletion.isPending} onClick={close} variant="link">
          <FormattedMessage {...messages.cancel} />
        </Button>
      </ModalFooter>
    </Modal>
  );
}
