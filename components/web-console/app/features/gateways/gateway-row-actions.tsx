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

import { messages } from "../../i18n/messages";
import {
  buildGatewayAddCommand,
  type GatewayConnection,
} from "./gateway-connections";
import { GatewayDeleteDialog } from "./gateway-delete-dialog";

export function GatewayRowActions({
  gateway,
  onDeleted,
}: {
  gateway: GatewayConnection;
  onDeleted: () => void;
}) {
  const intl = useIntl();
  const [isDeleteOpen, setIsDeleteOpen] = useState(false);
  const [isOpen, setIsOpen] = useState(false);
  const [copyResult, setCopyResult] = useState<"error" | "success">();

  const copyConnectionCommand = async () => {
    try {
      await navigator.clipboard.writeText(buildGatewayAddCommand(gateway));
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
          <DropdownItem isExternalLink rel="noreferrer" to={gateway.consoleUrl}>
            <FormattedMessage {...messages.openGatewayConsole} />
          </DropdownItem>
          <DropdownItem
            onClick={() => {
              void copyConnectionCommand();
            }}
          >
            <FormattedMessage {...messages.copyCliConnectionCommand} />
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
      <GatewayDeleteDialog
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
