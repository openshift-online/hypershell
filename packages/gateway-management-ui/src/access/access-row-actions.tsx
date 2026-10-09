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
  Tooltip,
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
  const { currentUser, gateways } = useGatewayUi();
  const queryClient = useQueryClient();
  const confirmTitleId = useId();
  const confirmBodyId = useId();
  const [isOpen, setIsOpen] = useState(false);
  const [pendingSelfRole, setPendingSelfRole] =
    useState<GatewayAccessRole | null>(null);
  const roles = assignableRoles(capabilities);
  // Changing your OWN access is easy to do by accident (e.g. demoting yourself),
  // so a self role change is confirmed before it is applied.
  const isSelf =
    currentUser?.username?.toLowerCase() === grant.username.toLowerCase();
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

  const label = roleLabel(intl.formatMessage, grant.role);
  const controlLabel = `${intl.formatMessage(messages.accessChangeRoleLabel)}: ${userDisplayName(grant)}`;

  // When the row cannot be managed (e.g. the last remaining owner, GAM-UI-08, or
  // an owner row for a non-owner caller), present the current role as a disabled
  // control with a tooltip explaining why, mirroring the disabled "Delete gateway"
  // affordance. isAriaDisabled keeps it focusable so the reason is announced.
  if (disabledReason !== null) {
    return (
      <Tooltip content={disabledReason}>
        <Button aria-label={disabledReason} isAriaDisabled variant="plain">
          {label}
        </Button>
      </Tooltip>
    );
  }

  const toggle = (toggleRef: React.Ref<MenuToggleElement>) => (
    <MenuToggle
      aria-label={controlLabel}
      isDisabled={mutation.isPending}
      isExpanded={isOpen}
      onClick={() => {
        setIsOpen((open) => !open);
      }}
      ref={toggleRef}
      variant="plainText"
    >
      {label}
    </MenuToggle>
  );

  return (
    <>
      <Select
        id={`gateway-access-role-${grant.userId}`}
        isOpen={isOpen}
        onOpenChange={setIsOpen}
        onSelect={(_event, value) => {
          setIsOpen(false);
          if (typeof value === "string" && value !== grant.role) {
            const nextRole = value as GatewayAccessRole;
            if (isSelf) {
              // Confirm before changing your own access (see isSelf above).
              setPendingSelfRole(nextRole);
            } else {
              mutation.mutate(nextRole);
            }
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
      {pendingSelfRole ? (
        <Modal
          aria-describedby={confirmBodyId}
          aria-labelledby={confirmTitleId}
          isOpen
          onClose={
            mutation.isPending
              ? undefined
              : () => {
                  setPendingSelfRole(null);
                }
          }
          variant={ModalVariant.small}
        >
          <ModalHeader
            labelId={confirmTitleId}
            title={intl.formatMessage(messages.accessSelfRoleChangeTitle)}
          />
          <ModalBody>
            <p id={confirmBodyId}>
              {intl.formatMessage(messages.accessSelfRoleChangeBody, {
                role: roleLabel(intl.formatMessage, pendingSelfRole),
              })}
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
                mutation.mutate(pendingSelfRole, {
                  onSuccess: () => {
                    setPendingSelfRole(null);
                  },
                });
              }}
              variant="primary"
            >
              {intl.formatMessage(messages.accessSelfRoleChangeConfirm)}
            </Button>
            <Button
              isDisabled={mutation.isPending}
              onClick={() => {
                mutation.reset();
                setPendingSelfRole(null);
              }}
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

  // The last remaining owner (and owner rows for non-owner callers) cannot be
  // removed (GAM-UI-08). Disable the action and explain why with a tooltip, as the
  // disabled "Delete gateway" affordance does. isAriaDisabled keeps it focusable.
  const removeButton = (
    <Button
      aria-label={removeLabel}
      isAriaDisabled={disabledReason !== null}
      isDanger
      onClick={() => {
        if (disabledReason !== null) {
          return;
        }
        setIsOpen(true);
      }}
      variant="link"
    >
      {intl.formatMessage(messages.accessRemove)}
    </Button>
  );

  return (
    <>
      {disabledReason !== null ? (
        <Tooltip content={disabledReason}>{removeButton}</Tooltip>
      ) : (
        removeButton
      )}
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
