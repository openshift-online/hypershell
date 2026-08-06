import {
  Alert,
  Button,
  Form,
  FormGroup,
  FormHelperText,
  HelperText,
  HelperTextItem,
  Modal,
  ModalBody,
  ModalFooter,
  ModalHeader,
  ModalVariant,
  Stack,
  StackItem,
  TextInput,
} from "@patternfly/react-core";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { FormattedMessage, useIntl } from "react-intl";

import { useGatewayUi } from "../gateway-ui-provider";
import { messages } from "../messages";
import { gatewayQueryKey } from "./gateway-data";

interface GatewayRenameDialogProps {
  gatewayId: string;
  gatewayName: string;
  onClose: () => void;
  onRenamed: (gatewayName: string) => void;
}

export function GatewayRenameDialog({
  gatewayId,
  gatewayName,
  onClose,
  onRenamed,
}: GatewayRenameDialogProps) {
  const intl = useIntl();
  const { gateways } = useGatewayUi();
  const queryClient = useQueryClient();
  const fieldId = "gateway-rename-name";
  const formId = useId();
  const titleId = useId();
  const [name, setName] = useState(gatewayName);
  const [showRequired, setShowRequired] = useState(false);
  const trimmedName = name.trim();
  const rename = useMutation({
    mutationFn: () => gateways.renameGateway(gatewayId, trimmedName),
    onSuccess: async (gateway) => {
      queryClient.setQueryData(gatewayQueryKey(gatewayId), gateway);
      onRenamed(gateway.name);
      await queryClient.invalidateQueries({
        exact: true,
        queryKey: ["gateways"],
      });
    },
  });

  const close = () => {
    rename.reset();
    onClose();
  };
  const submit = (event: React.SyntheticEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!trimmedName) {
      setShowRequired(true);
      return;
    }
    rename.mutate();
  };

  return (
    <Modal
      aria-labelledby={titleId}
      elementToFocus={`#${fieldId}`}
      isOpen
      onClose={rename.isPending ? undefined : close}
      variant={ModalVariant.small}
    >
      <ModalHeader
        labelId={titleId}
        title={intl.formatMessage(messages.renameGatewayTitle, {
          gatewayName,
        })}
      />
      <ModalBody>
        <Stack hasGutter>
          {rename.isError ? (
            <StackItem>
              <Alert
                isInline
                title={intl.formatMessage(messages.gatewayRenameError)}
                variant="danger"
              >
                <FormattedMessage {...messages.gatewayRenameErrorBody} />
              </Alert>
            </StackItem>
          ) : null}
          <StackItem>
            <Form id={formId} onSubmit={submit}>
              <FormGroup
                fieldId={fieldId}
                isRequired
                label={intl.formatMessage(messages.gatewayName)}
              >
                <TextInput
                  aria-describedby={
                    showRequired ? `${fieldId}-helper` : undefined
                  }
                  id={fieldId}
                  isDisabled={rename.isPending}
                  isRequired
                  name="name"
                  onChange={(_event, value) => {
                    setName(value);
                    setShowRequired(false);
                    if (rename.isError) {
                      rename.reset();
                    }
                  }}
                  validated={showRequired ? "error" : "default"}
                  value={name}
                />
                {showRequired ? (
                  <FormHelperText>
                    <HelperText>
                      <HelperTextItem id={`${fieldId}-helper`} variant="error">
                        <FormattedMessage {...messages.requiredField} />
                      </HelperTextItem>
                    </HelperText>
                  </FormHelperText>
                ) : null}
              </FormGroup>
            </Form>
          </StackItem>
        </Stack>
      </ModalBody>
      <ModalFooter>
        <Button
          form={formId}
          isDisabled={rename.isPending || trimmedName === gatewayName}
          type="submit"
          variant="primary"
          {...(rename.isPending
            ? {
                isLoading: true,
                spinnerAriaValueText: intl.formatMessage(
                  messages.renamingGateway,
                ),
              }
            : {})}
        >
          <FormattedMessage {...messages.renameGateway} />
        </Button>
        <Button isDisabled={rename.isPending} onClick={close} variant="link">
          <FormattedMessage {...messages.cancel} />
        </Button>
      </ModalFooter>
    </Modal>
  );
}
