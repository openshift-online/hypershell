import {
  Alert,
  Button,
  Modal,
  ModalBody,
  ModalFooter,
  ModalHeader,
  ModalVariant,
  Select,
  SelectList,
  SelectOption,
  MenuToggle,
  type MenuToggleElement,
} from "@patternfly/react-core";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { useIntl } from "react-intl";

import type {
  GatewayAccessCapabilities,
  GatewayAccessGrantRecord,
  GatewayAccessRole,
} from "../application/gateway-types";
import { useGatewayUi } from "../gateway-ui-provider";
import { messages } from "../messages";
import { accessListQueryRoot } from "./access-data";
import { actionErrorMessage, assignableRoles, roleLabel } from "./access-role";

function userDisplayName(grant: GatewayAccessGrantRecord): string {
  return grant.name?.trim() ? grant.name : grant.username;
}

export function AccessRoleControl({
  capabilities,
  disabledReason,
  gatewayId,
  grant,
  onActionError,
}: {
  capabilities: GatewayAccessCapabilities;
  disabledReason: string | null;
  gatewayId: string;
  grant: GatewayAccessGrantRecord;
  onActionError: (message: string | null) => void;
}) {
  const intl = useIntl();
  const { gateways } = useGatewayUi();
  const queryClient = useQueryClient();
  const [isOpen, setIsOpen] = useState(false);
  const roles = assignableRoles(capabilities);
  const mutation = useMutation({
    mutationFn: (role: GatewayAccessRole) =>
      gateways.changeGatewayAccessRole(gatewayId, grant.userId, role),
    onError: (error) => {
      onActionError(actionErrorMessage(intl.formatMessage, error));
    },
    onSuccess: async () => {
      onActionError(null);
      await queryClient.invalidateQueries({
        queryKey: accessListQueryRoot(gatewayId),
      });
    },
    retry: false,
  });

  const disabled = disabledReason !== null || mutation.isPending;
  const label = roleLabel(intl.formatMessage, grant.role);
  const controlLabel = `${intl.formatMessage(messages.accessChangeRoleLabel)}: ${userDisplayName(grant)}`;

  const toggle = (toggleRef: React.Ref<MenuToggleElement>) => (
    <MenuToggle
      aria-label={disabledReason ?? controlLabel}
      isDisabled={disabled}
      isExpanded={isOpen}
      onClick={() => {
        setIsOpen((open) => !open);
      }}
      ref={toggleRef}
      title={disabledReason ?? undefined}
      variant="plainText"
    >
      {label}
    </MenuToggle>
  );

  return (
    <Select
      id={`gateway-access-role-${grant.userId}`}
      isOpen={isOpen}
      onOpenChange={setIsOpen}
      onSelect={(_event, value) => {
        setIsOpen(false);
        if (typeof value === "string" && value !== grant.role) {
          mutation.mutate(value as GatewayAccessRole);
        }
      }}
      selected={grant.role}
      toggle={toggle}
    >
      <SelectList>
        {roles.map((role) => (
          <SelectOption
            isSelected={role === grant.role}
            key={role}
            value={role}
          >
            {roleLabel(intl.formatMessage, role)}
          </SelectOption>
        ))}
      </SelectList>
    </Select>
  );
}

export function AccessRemoveAction({
  disabledReason,
  gatewayId,
  grant,
}: {
  disabledReason: string | null;
  gatewayId: string;
  grant: GatewayAccessGrantRecord;
}) {
  const intl = useIntl();
  const { gateways } = useGatewayUi();
  const queryClient = useQueryClient();
  const descriptionId = useId();
  const titleId = useId();
  const [isOpen, setIsOpen] = useState(false);
  const mutation = useMutation({
    mutationFn: () => gateways.revokeGatewayAccess(gatewayId, grant.userId),
    onSuccess: async () => {
      setIsOpen(false);
      await queryClient.invalidateQueries({
        queryKey: accessListQueryRoot(gatewayId),
      });
    },
    retry: false,
  });
  const close = () => {
    mutation.reset();
    setIsOpen(false);
  };
  const removeLabel =
    disabledReason ??
    `${intl.formatMessage(messages.accessRemove)}: ${userDisplayName(grant)}`;

  return (
    <>
      <Button
        aria-label={removeLabel}
        isDanger
        isDisabled={disabledReason !== null}
        onClick={() => {
          setIsOpen(true);
        }}
        title={disabledReason ?? undefined}
        variant="link"
      >
        {intl.formatMessage(messages.accessRemove)}
      </Button>
      {isOpen ? (
        <Modal
          aria-describedby={descriptionId}
          aria-labelledby={titleId}
          isOpen
          onClose={mutation.isPending ? undefined : close}
          variant={ModalVariant.small}
        >
          <ModalHeader
            labelId={titleId}
            title={intl.formatMessage(messages.accessRemoveTitle, {
              userName: userDisplayName(grant),
            })}
          />
          <ModalBody>
            <p id={descriptionId}>
              {intl.formatMessage(messages.accessRemoveBody)}
            </p>
            {mutation.isError ? (
              <Alert
                isInline
                title={actionErrorMessage(intl.formatMessage, mutation.error)}
                variant="danger"
              />
            ) : null}
          </ModalBody>
          <ModalFooter>
            <Button
              isDanger
              isDisabled={mutation.isPending}
              isLoading={mutation.isPending}
              onClick={() => {
                mutation.mutate();
              }}
              spinnerAriaValueText={intl.formatMessage(messages.accessRemoving)}
              variant="primary"
            >
              {intl.formatMessage(messages.accessRemove)}
            </Button>
            <Button
              isDisabled={mutation.isPending}
              onClick={close}
              variant="link"
            >
              {intl.formatMessage(messages.cancel)}
            </Button>
          </ModalFooter>
        </Modal>
      ) : null}
    </>
  );
}
