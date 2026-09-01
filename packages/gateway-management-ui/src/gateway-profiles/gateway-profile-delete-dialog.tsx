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

import { GatewayProfileOperationError } from "../application/gateway-profile-types";
import { useGatewayProfileUi } from "../gateway-profile-ui-provider";
import { gatewayProfileMessages } from "../gateway-profile-messages";
import { messages } from "../messages";
import {
  gatewayProfileListQueryRoot,
  gatewayProfileQueryKey,
} from "./gateway-profile-data";

interface GatewayProfileDeleteDialogProps {
  gatewayProfileId: string;
  gatewayProfileName: string;
  isOpen: boolean;
  onClose: () => void;
  onDeleted: () => void;
}

function isConflict(error: unknown): boolean {
  return (
    error instanceof GatewayProfileOperationError && error.kind === "conflict"
  );
}

export function GatewayProfileDeleteDialog({
  gatewayProfileId,
  gatewayProfileName,
  isOpen,
  onClose,
  onDeleted,
}: GatewayProfileDeleteDialogProps) {
  const intl = useIntl();
  const { gatewayProfiles } = useGatewayProfileUi();
  const queryClient = useQueryClient();
  const descriptionId = useId();
  const titleId = useId();
  const deletion = useMutation({
    mutationFn: () => gatewayProfiles.removeGatewayProfile(gatewayProfileId),
    onSuccess: async () => {
      queryClient.removeQueries({
        exact: true,
        queryKey: gatewayProfileQueryKey(gatewayProfileId),
      });
      onDeleted();
      await queryClient.invalidateQueries({
        queryKey: gatewayProfileListQueryRoot,
      });
    },
  });

  const close = () => {
    deletion.reset();
    onClose();
  };

  const conflict = deletion.isError && isConflict(deletion.error);

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
        title={intl.formatMessage(
          gatewayProfileMessages.deleteGatewayProfileTitle,
          { gatewayProfileName },
        )}
      />
      <ModalBody>
        <Stack hasGutter>
          <StackItem id={descriptionId}>
            <FormattedMessage
              {...gatewayProfileMessages.deleteGatewayProfileConfirmation}
              values={{ gatewayProfileName }}
            />
          </StackItem>
          {deletion.isError ? (
            <StackItem>
              <Alert
                isInline
                title={intl.formatMessage(
                  conflict
                    ? gatewayProfileMessages.gatewayProfileDeleteConflict
                    : gatewayProfileMessages.gatewayProfileDeleteError,
                )}
                variant="danger"
              >
                <FormattedMessage
                  {...(conflict
                    ? gatewayProfileMessages.gatewayProfileDeleteConflictBody
                    : gatewayProfileMessages.gatewayProfileDeleteErrorBody)}
                />
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
          spinnerAriaValueText={intl.formatMessage(
            gatewayProfileMessages.deletingGatewayProfile,
          )}
          variant="danger"
        >
          <FormattedMessage {...gatewayProfileMessages.deleteGatewayProfile} />
        </Button>
        <Button isDisabled={deletion.isPending} onClick={close} variant="link">
          <FormattedMessage {...messages.cancel} />
        </Button>
      </ModalFooter>
    </Modal>
  );
}
