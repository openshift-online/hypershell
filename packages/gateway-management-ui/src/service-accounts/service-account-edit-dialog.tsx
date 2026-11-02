import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useIntl } from "react-intl";

import {
  Alert,
  Button,
  Form,
  FormGroup,
  MenuToggle,
  Modal,
  ModalBody,
  ModalFooter,
  ModalHeader,
  Select,
  SelectList,
  SelectOption,
  TextArea,
  ModalVariant,
} from "@patternfly/react-core";

import {
  GatewayOperationError,
  type OpenShellGatewayServiceAccountCapabilities,
  type OpenShellGatewayServiceAccountRecord,
  type OpenShellGatewayServiceAccountRole,
} from "../application/gateway-types";
import { useGatewayUi } from "../gateway-ui-provider";
import { messages } from "../messages";
import {
  serviceAccountListQueryRoot,
  serviceAccountQueryKey,
} from "./service-account-data";

/**
 * Edit dialog for the in-place role update. Collects the OpenShell role and an
 * optional replacement description for a ready or degraded service account.
 * The role options come from the server-provided capabilities, but the server
 * remains authoritative: a role above the account creator's current gateway
 * role is rejected with role_not_allowed and explained locally.
 */
export function ServiceAccountEditDialog({
  account,
  capabilities,
  gatewayId,
  isOpen,
  onClose,
}: {
  account: OpenShellGatewayServiceAccountRecord;
  capabilities: OpenShellGatewayServiceAccountCapabilities;
  gatewayId: string;
  isOpen: boolean;
  onClose: () => void;
}) {
  const intl = useIntl();
  const { gateways } = useGatewayUi();
  const queryClient = useQueryClient();
  const [role, setRole] = useState<OpenShellGatewayServiceAccountRole>(
    account.role,
  );
  const [description, setDescription] = useState(account.description ?? "");
  const [isRoleOpen, setIsRoleOpen] = useState(false);

  const mutation = useMutation({
    mutationFn: async () => {
      await gateways.updateOpenShellGatewayServiceAccountRole(
        gatewayId,
        account.id,
        {
          role,
          ...(description !== (account.description ?? "")
            ? { description }
            : {}),
        },
      );
    },
    onSuccess: async () => {
      onClose();
      await queryClient.invalidateQueries({
        queryKey: serviceAccountListQueryRoot(gatewayId),
      });
      await queryClient.invalidateQueries({
        queryKey: serviceAccountQueryKey(gatewayId, account.id),
      });
    },
    retry: false,
  });

  const close = () => {
    mutation.reset();
    setRole(account.role);
    setDescription(account.description ?? "");
    setIsRoleOpen(false);
    onClose();
  };

  const roleNotAllowed =
    mutation.error instanceof GatewayOperationError &&
    mutation.error.code === "role-not-allowed";

  return (
    <Modal
      aria-labelledby="service-account-edit-title"
      isOpen={isOpen}
      onClose={mutation.isPending ? undefined : close}
      variant={ModalVariant.small}
    >
      <ModalHeader
        labelId="service-account-edit-title"
        title={intl.formatMessage(messages.editServiceAccountRoleTitle, {
          serviceAccountName: account.name,
        })}
      />
      <ModalBody>
        <p>
          {intl.formatMessage(messages.editServiceAccountRoleIntroduction, {
            serviceAccountName: account.name,
          })}
        </p>
        <Form
          onSubmit={(event) => {
            event.preventDefault();
            mutation.mutate();
          }}
        >
          <FormGroup
            fieldId="service-account-edit-role"
            isRequired
            label={intl.formatMessage(messages.serviceAccountRole)}
          >
            <Select
              id="service-account-edit-role"
              isOpen={isRoleOpen}
              onOpenChange={setIsRoleOpen}
              onSelect={(_event, value) => {
                if (
                  typeof value === "string" &&
                  capabilities.allowedRoles.includes(
                    value as OpenShellGatewayServiceAccountRole,
                  )
                ) {
                  setRole(value as OpenShellGatewayServiceAccountRole);
                }
                setIsRoleOpen(false);
              }}
              selected={role}
              toggle={(toggleRef) => (
                <MenuToggle
                  aria-label={intl.formatMessage(messages.serviceAccountRole)}
                  isDisabled={mutation.isPending}
                  isExpanded={isRoleOpen}
                  isFullWidth
                  onClick={() => {
                    setIsRoleOpen((open) => !open);
                  }}
                  ref={toggleRef}
                >
                  {role}
                </MenuToggle>
              )}
            >
              <SelectList>
                {capabilities.allowedRoles.map((option) => (
                  <SelectOption
                    description={intl.formatMessage(
                      option === "openshell-admin"
                        ? messages.openshellAdminRoleDescription
                        : messages.openshellUserRoleDescription,
                    )}
                    key={option}
                    value={option}
                  >
                    {option}
                  </SelectOption>
                ))}
              </SelectList>
            </Select>
          </FormGroup>
          <FormGroup
            fieldId="service-account-edit-description"
            label={intl.formatMessage(messages.serviceAccountDescription)}
          >
            <TextArea
              id="service-account-edit-description"
              isDisabled={mutation.isPending}
              onChange={(_event, value) => {
                setDescription(value);
              }}
              rows={3}
              value={description}
            />
          </FormGroup>
        </Form>
        <Alert
          isInline
          title={intl.formatMessage(messages.editServiceAccountRoleEffect)}
          variant="info"
        >
          {intl.formatMessage(messages.editServiceAccountRoleEffectBody)}
        </Alert>
        {roleNotAllowed ? (
          <Alert
            isInline
            title={intl.formatMessage(messages.serviceAccountRoleNotAllowed)}
            variant="danger"
          >
            {intl.formatMessage(messages.serviceAccountRoleNotAllowedBody)}
          </Alert>
        ) : mutation.isError ? (
          <Alert
            isInline
            title={intl.formatMessage(messages.serviceAccountActionError)}
            variant="danger"
          >
            {intl.formatMessage(messages.serviceAccountActionErrorBody)}
          </Alert>
        ) : null}
      </ModalBody>
      <ModalFooter>
        <Button
          isDisabled={mutation.isPending}
          isLoading={mutation.isPending}
          onClick={() => {
            mutation.mutate();
          }}
          spinnerAriaValueText={intl.formatMessage(
            messages.editServiceAccountRole,
          )}
          variant="primary"
        >
          {intl.formatMessage(messages.editServiceAccountRoleSave)}
        </Button>
        <Button isDisabled={mutation.isPending} onClick={close} variant="link">
          {intl.formatMessage(messages.cancel)}
        </Button>
      </ModalFooter>
    </Modal>
  );
}
