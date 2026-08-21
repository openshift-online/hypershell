import {
  Alert,
  Button,
  Dropdown,
  DropdownItem,
  DropdownList,
  MenuToggle,
  Modal,
  ModalBody,
  ModalFooter,
  ModalHeader,
  ModalVariant,
  Spinner,
} from "@patternfly/react-core";
import { EllipsisVIcon } from "@patternfly/react-icons";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { useIntl } from "react-intl";

import type { OpenShellGatewayServiceAccountRecord } from "../application/gateway-types";
import { useGatewayUi } from "../gateway-ui-provider";
import { messages } from "../messages";
import {
  serviceAccountListQueryRoot,
  serviceAccountQueryKey,
} from "./service-account-data";
import { ServiceAccountSetupView } from "./service-account-setup";

type LifecycleAction = "delete" | "revoke";

function ServiceAccountLifecycleDialog({
  account,
  action,
  gatewayId,
  isOpen,
  onClose,
}: {
  account: OpenShellGatewayServiceAccountRecord;
  action: LifecycleAction;
  gatewayId: string;
  isOpen: boolean;
  onClose: () => void;
}) {
  const intl = useIntl();
  const { gateways } = useGatewayUi();
  const queryClient = useQueryClient();
  const descriptionId = useId();
  const titleId = useId();
  const mutation = useMutation({
    mutationFn: async () => {
      if (action === "revoke") {
        await gateways.revokeOpenShellGatewayServiceAccount(
          gatewayId,
          account.id,
        );
      } else {
        await gateways.deleteOpenShellGatewayServiceAccount(
          gatewayId,
          account.id,
        );
      }
    },
    onSuccess: async () => {
      onClose();
      await queryClient.invalidateQueries({
        queryKey: serviceAccountListQueryRoot(gatewayId),
      });
    },
    retry: false,
  });
  const close = () => {
    mutation.reset();
    onClose();
  };

  return (
    <Modal
      aria-describedby={descriptionId}
      aria-labelledby={titleId}
      isOpen={isOpen}
      onClose={mutation.isPending ? undefined : close}
      variant={ModalVariant.small}
    >
      <ModalHeader
        labelId={titleId}
        title={intl.formatMessage(
          action === "revoke"
            ? messages.revokeServiceAccountTitle
            : messages.deleteServiceAccountTitle,
          { serviceAccountName: account.name },
        )}
      />
      <ModalBody>
        <p id={descriptionId}>
          {intl.formatMessage(
            action === "revoke"
              ? messages.revokeServiceAccountBody
              : messages.deleteServiceAccountBody,
          )}
        </p>
        {mutation.isError ? (
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
            action === "revoke"
              ? messages.revokingServiceAccount
              : messages.deletingServiceAccount,
          )}
          variant={action === "delete" ? "danger" : "primary"}
        >
          {intl.formatMessage(
            action === "revoke"
              ? messages.revokeServiceAccount
              : messages.deleteServiceAccount,
          )}
        </Button>
        <Button isDisabled={mutation.isPending} onClick={close} variant="link">
          {intl.formatMessage(messages.cancel)}
        </Button>
      </ModalFooter>
    </Modal>
  );
}

function ExistingServiceAccountSetupDialog({
  account,
  gatewayId,
  isOpen,
  onClose,
}: {
  account: OpenShellGatewayServiceAccountRecord;
  gatewayId: string;
  isOpen: boolean;
  onClose: () => void;
}) {
  const intl = useIntl();
  const { gateways } = useGatewayUi();
  const titleId = useId();
  const detail = useQuery({
    enabled: isOpen,
    queryFn: ({ signal }) =>
      gateways.getOpenShellGatewayServiceAccount(gatewayId, account.id, signal),
    queryKey: serviceAccountQueryKey(gatewayId, account.id),
    retry: false,
    staleTime: 30_000,
  });

  return (
    <Modal
      aria-labelledby={titleId}
      isOpen={isOpen}
      onClose={onClose}
      variant={ModalVariant.large}
    >
      <ModalHeader
        labelId={titleId}
        title={intl.formatMessage(messages.serviceAccountSetupTitle, {
          serviceAccountName: account.name,
        })}
      />
      <ModalBody>
        {detail.isPending ? (
          <Spinner
            aria-label={intl.formatMessage(messages.loadingServiceAccount)}
          />
        ) : detail.isError ? (
          <Alert
            actionLinks={
              <Button
                isInline
                onClick={() => void detail.refetch()}
                variant="link"
              >
                {intl.formatMessage(messages.retry)}
              </Button>
            }
            isInline
            title={intl.formatMessage(messages.serviceAccountsLoadError)}
            variant="danger"
          />
        ) : (
          <ServiceAccountSetupView
            account={detail.data.serviceAccount}
            connection={detail.data.connection}
            isAcknowledged={false}
            onAcknowledgedChange={() => undefined}
          />
        )}
      </ModalBody>
      <ModalFooter>
        <Button onClick={onClose} variant="primary">
          {intl.formatMessage(messages.closeSetup)}
        </Button>
      </ModalFooter>
    </Modal>
  );
}

export function ServiceAccountRowActions({
  account,
  gatewayId,
}: {
  account: OpenShellGatewayServiceAccountRecord;
  gatewayId: string;
}) {
  const intl = useIntl();
  const [action, setAction] = useState<LifecycleAction>();
  const [isMenuOpen, setIsMenuOpen] = useState(false);
  const [isSetupOpen, setIsSetupOpen] = useState(false);
  const canRevoke = !["deleting", "expired", "revoked", "revoking"].includes(
    account.status,
  );

  return (
    <>
      <Dropdown
        isOpen={isMenuOpen}
        onOpenChange={setIsMenuOpen}
        onSelect={() => {
          setIsMenuOpen(false);
        }}
        popperProps={{ position: "right" }}
        shouldFocusToggleOnSelect
        toggle={(toggleRef) => (
          <MenuToggle
            aria-label={intl.formatMessage(messages.serviceAccountRowActions, {
              serviceAccountName: account.name,
            })}
            isExpanded={isMenuOpen}
            onClick={() => {
              setIsMenuOpen((open) => !open);
            }}
            ref={toggleRef}
            variant="plain"
          >
            <EllipsisVIcon aria-hidden />
          </MenuToggle>
        )}
      >
        <DropdownList>
          {account.status === "ready" ? (
            <DropdownItem
              onClick={() => {
                setIsSetupOpen(true);
              }}
            >
              {intl.formatMessage(messages.viewSetupInstructions)}
            </DropdownItem>
          ) : null}
          {canRevoke ? (
            <DropdownItem
              onClick={() => {
                setAction("revoke");
              }}
            >
              {intl.formatMessage(messages.revokeServiceAccount)}
            </DropdownItem>
          ) : null}
          {account.status !== "deleting" ? (
            <DropdownItem
              isDanger
              onClick={() => {
                setAction("delete");
              }}
            >
              {intl.formatMessage(messages.deleteServiceAccount)}
            </DropdownItem>
          ) : null}
        </DropdownList>
      </Dropdown>
      <ExistingServiceAccountSetupDialog
        account={account}
        gatewayId={gatewayId}
        isOpen={isSetupOpen}
        onClose={() => {
          setIsSetupOpen(false);
        }}
      />
      {action ? (
        <ServiceAccountLifecycleDialog
          account={account}
          action={action}
          gatewayId={gatewayId}
          isOpen
          onClose={() => {
            setAction(undefined);
          }}
        />
      ) : null}
    </>
  );
}
