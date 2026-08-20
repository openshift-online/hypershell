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
  onDeleted,
  onRenamed,
}: {
  gateway: GatewayConnection;
  onDeleted: () => void;
  onRenamed: (gatewayName: string) => void;
}) {
  const intl = useIntl();
  const [isDeleteOpen, setIsDeleteOpen] = useState(false);
  const [isOpen, setIsOpen] = useState(false);
  const [isRenameOpen, setIsRenameOpen] = useState(false);
  const [copyResult, setCopyResult] = useState<"error" | "success">();
  const connectionCommand = buildGatewayAddCommand(gateway);

  const copyConnectionCommand = async () => {
    if (!connectionCommand) {
      return;
    }
    try {
      await navigator.clipboard.writeText(connectionCommand);
      setCopyResult("success");
    } catch {
      setCopyResult("error");
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
                void copyConnectionCommand();
              }}
            >
              <FormattedMessage {...messages.copyCliConnectionCommand} />
            </DropdownItem>
          ) : null}
          <DropdownItem
            onClick={() => {
              setIsRenameOpen(true);
            }}
          >
            <FormattedMessage {...messages.renameGateway} />
          </DropdownItem>
          <DropdownItem
            isDanger
            onClick={() => {
              setIsDeleteOpen(true);
            }}
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
      {copyResult ? (
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
                  setCopyResult(undefined);
                }}
              />
            }
            onTimeout={() => {
              setCopyResult(undefined);
            }}
            timeout={8000}
            title={intl.formatMessage(
              copyResult === "success"
                ? messages.cliConnectionCommandCopied
                : messages.cliConnectionCommandCopyFailed,
              { gatewayName: gateway.name },
            )}
            variant={copyResult === "success" ? "success" : "danger"}
          />
        </AlertGroup>
      ) : null}
    </>
  );
}
