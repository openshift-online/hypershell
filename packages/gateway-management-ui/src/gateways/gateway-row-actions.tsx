import {
  Alert,
  AlertActionCloseButton,
  AlertGroup,
  Dropdown,
  DropdownItem,
  DropdownList,
  MenuToggle,
} from "@patternfly/react-core";
import { EllipsisVIcon } from "@patternfly/react-icons";
import { useState } from "react";
import { FormattedMessage, useIntl } from "react-intl";

import { messages } from "../messages";
import {
  buildGatewayAddCommand,
  type GatewayConnection,
} from "./gateway-connections";
import { GatewayDeleteDialog } from "./gateway-delete-dialog";
import { GatewayRenameDialog } from "./gateway-rename-dialog";
import styles from "./gateway-row-actions.module.css";

export function GatewayRowActions({
  gateway,
  onConnectionCommand,
  onDeleted,
  onRenamed,
}: {
  gateway: GatewayConnection;
  onConnectionCommand: () => void;
  onDeleted: () => void;
  onRenamed: (gatewayName: string) => void;
}) {
  const intl = useIntl();
  const [isDeleteOpen, setIsDeleteOpen] = useState(false);
  const [isOpen, setIsOpen] = useState(false);
  const [isRenameOpen, setIsRenameOpen] = useState(false);
  const [endpointCopyResult, setEndpointCopyResult] = useState<
    "error" | "success"
  >();
  const connectionCommand = buildGatewayAddCommand(gateway);
  // Disable delete/rename only when the server has explicitly told us the caller
  // cannot. An absent capability (older server) leaves it enabled; the API 403
  // remains authoritative either way.
  const canDelete = gateway.canDelete !== false;
  const canEdit = gateway.canEdit !== false;

  const copyEndpoint = async () => {
    if (!gateway.endpoint) {
      return;
    }
    try {
      await navigator.clipboard.writeText(gateway.endpoint);
      setEndpointCopyResult("success");
    } catch {
      setEndpointCopyResult("error");
    }
  };

  return (
    <>
      <Dropdown
        isOpen={isOpen}
        onOpenChange={setIsOpen}
        onSelect={() => {
          setIsOpen(false);
        }}
        popperProps={{ position: "right" }}
        shouldFocusToggleOnSelect
        toggle={(toggleRef) => (
          <MenuToggle
            aria-label={intl.formatMessage(messages.gatewayRowActions, {
              gatewayName: gateway.name,
            })}
            isExpanded={isOpen}
            onClick={() => {
              setIsOpen((open) => !open);
            }}
            ref={toggleRef}
            variant="plain"
          >
            <EllipsisVIcon aria-hidden="true" />
          </MenuToggle>
        )}
      >
        <DropdownList>
          {gateway.consoleUrl ? (
            <DropdownItem
              className={styles.consoleLink}
              isExternalLink
              rel="noreferrer"
              to={gateway.consoleUrl}
            >
              <FormattedMessage {...messages.openGatewayConsole} />
            </DropdownItem>
          ) : null}
          {connectionCommand ? (
            <DropdownItem
              onClick={() => {
                onConnectionCommand();
              }}
            >
              <FormattedMessage {...messages.connectionCommandAction} />
            </DropdownItem>
          ) : null}
          {gateway.endpoint ? (
            <DropdownItem
              onClick={() => {
                void copyEndpoint();
              }}
            >
              <FormattedMessage {...messages.copyEndpointAction} />
            </DropdownItem>
          ) : null}
          <DropdownItem
            isAriaDisabled={!canEdit}
            onClick={() => {
              if (!canEdit) {
                return;
              }
              setIsRenameOpen(true);
            }}
            tooltipProps={
              canEdit
                ? undefined
                : {
                    content: intl.formatMessage(
                      messages.renameGatewayNotPermitted,
                    ),
                  }
            }
          >
            <FormattedMessage {...messages.renameGateway} />
          </DropdownItem>
          <DropdownItem
            isAriaDisabled={!canDelete}
            isDanger={canDelete}
            onClick={() => {
              if (!canDelete) {
                return;
              }
              setIsDeleteOpen(true);
            }}
            tooltipProps={
              canDelete
                ? undefined
                : {
                    content: intl.formatMessage(
                      messages.deleteGatewayNotPermitted,
                    ),
                  }
            }
          >
            <FormattedMessage {...messages.deleteGateway} />
          </DropdownItem>
        </DropdownList>
      </Dropdown>
      {isRenameOpen ? (
        <GatewayRenameDialog
          gatewayId={gateway.id}
          gatewayName={gateway.name}
          onClose={() => {
            setIsRenameOpen(false);
          }}
          onRenamed={(gatewayName) => {
            setIsRenameOpen(false);
            onRenamed(gatewayName);
          }}
        />
      ) : null}
      <GatewayDeleteDialog
        activeSandboxCount={gateway.activeSandboxCount}
        gatewayId={gateway.id}
        gatewayName={gateway.name}
        isOpen={isDeleteOpen}
        onClose={() => {
          setIsDeleteOpen(false);
        }}
        onDeleted={() => {
          setIsDeleteOpen(false);
          onDeleted();
        }}
      />
      {endpointCopyResult ? (
        <AlertGroup
          aria-label={intl.formatMessage(messages.notifications)}
          isLiveRegion
          isToast
        >
          <Alert
            actionClose={
              <AlertActionCloseButton
                aria-label={intl.formatMessage(messages.close)}
                onClose={() => {
                  setEndpointCopyResult(undefined);
                }}
              />
            }
            onTimeout={() => {
              setEndpointCopyResult(undefined);
            }}
            timeout={8000}
            title={intl.formatMessage(
              endpointCopyResult === "success"
                ? messages.gatewayEndpointCopied
                : messages.gatewayEndpointCopyFailed,
              { gatewayName: gateway.name },
            )}
            variant={endpointCopyResult === "success" ? "success" : "danger"}
          />
        </AlertGroup>
      ) : null}
    </>
  );
}
