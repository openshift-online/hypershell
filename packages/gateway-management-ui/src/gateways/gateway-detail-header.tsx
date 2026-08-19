import {
  ActionList,
  ActionListItem,
  Button,
  ClipboardCopy,
  ClipboardCopyButton,
  CodeBlock,
  CodeBlockAction,
  CodeBlockCode,
  Content,
  Divider,
  Dropdown,
  DropdownItem,
  DropdownList,
  Flex,
  FlexItem,
  MenuToggle,
  Title,
} from "@patternfly/react-core";
import { ExternalLinkAltIcon } from "@patternfly/react-icons";
import { type ReactNode, useId, useState } from "react";
import { FormattedMessage, useIntl } from "react-intl";

import { messages } from "../messages";
import {
  buildGatewayAddCommand,
  type GatewayConnection,
} from "./gateway-connections";
import { GatewayDeleteDialog } from "./gateway-delete-dialog";
import { GatewayRenameDialog } from "./gateway-rename-dialog";
import { GatewayStatus } from "./gateway-status";
import styles from "./gateway-detail-header.module.css";

export function GatewayCliCopy({ gateway }: { gateway: GatewayConnection }) {
  const intl = useIntl();
  const id = useId();
  const [copied, setCopied] = useState(false);
  const connectionCommand = buildGatewayAddCommand(gateway);

  if (!connectionCommand) {
    return null;
  }

  const handleCopy = () => {
    void navigator.clipboard.writeText(connectionCommand);
    setCopied(true);
  };

  return (
    <CodeBlock
      actions={
        <CodeBlockAction>
          <ClipboardCopyButton
            aria-label={intl.formatMessage(messages.copyConnectionCommand, {
              gatewayName: gateway.name,
            })}
            exitDelay={copied ? 1500 : 600}
            id={`${id}-copy-button`}
            maxWidth="110px"
            onClick={handleCopy}
            onTooltipHidden={() => {
              setCopied(false);
            }}
            variant="plain"
          >
            {copied
              ? intl.formatMessage(messages.copied)
              : intl.formatMessage(messages.copy)}
          </ClipboardCopyButton>
        </CodeBlockAction>
      }
    >
      <CodeBlockCode id={id}>{connectionCommand}</CodeBlockCode>
    </CodeBlock>
  );
}

export function GatewayEndpointCopy({
  gateway,
}: {
  gateway: GatewayConnection;
}) {
  const intl = useIntl();

  if (!gateway.endpoint) {
    return null;
  }

  return (
    <ClipboardCopy
      clickTip={intl.formatMessage(messages.copied)}
      copyAriaLabel={intl.formatMessage(messages.copyGatewayEndpoint, {
        gatewayName: gateway.name,
      })}
      hoverTip={intl.formatMessage(messages.copy)}
      isCode
      isReadOnly
    >
      {gateway.endpoint}
    </ClipboardCopy>
  );
}

function GatewayDetailActions({
  gateway,
  onDeleted,
  onRenamed,
}: {
  gateway: GatewayConnection;
  onDeleted: () => void;
  onRenamed: (gatewayName: string) => void;
}) {
  const intl = useIntl();
  const [isActionsOpen, setIsActionsOpen] = useState(false);
  const [isDeleteOpen, setIsDeleteOpen] = useState(false);
  const [isRenameOpen, setIsRenameOpen] = useState(false);

  return (
    <>
      <ActionList>
        {gateway.consoleUrl ? (
          <ActionListItem>
            <Button
              aria-label={intl.formatMessage(messages.openGatewayConsoleFor, {
                gatewayName: gateway.name,
              })}
              component="a"
              href={gateway.consoleUrl}
              icon={<ExternalLinkAltIcon />}
              iconPosition="end"
              rel="noreferrer"
              target="_blank"
              variant="primary"
            >
              <FormattedMessage {...messages.openGatewayConsole} />
            </Button>
          </ActionListItem>
        ) : null}
        <ActionListItem>
          <Dropdown
            isOpen={isActionsOpen}
            onOpenChange={setIsActionsOpen}
            onSelect={() => {
              setIsActionsOpen(false);
            }}
            shouldFocusToggleOnSelect
            toggle={(toggleRef) => (
              <MenuToggle
                className={styles.actionsToggle}
                isExpanded={isActionsOpen}
                onClick={() => {
                  setIsActionsOpen((open) => !open);
                }}
                ref={toggleRef}
                variant="secondary"
              >
                <FormattedMessage {...messages.actions} />
              </MenuToggle>
            )}
          >
            <DropdownList>
              <DropdownItem
                onClick={() => {
                  setIsRenameOpen(true);
                }}
              >
                <FormattedMessage {...messages.renameGateway} />
              </DropdownItem>
              <Divider component="li" />
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
        </ActionListItem>
      </ActionList>
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
    </>
  );
}

export function GatewayDetailHeader({
  description,
  gateway,
  onDeleted,
  onRenamed,
}: {
  description?: ReactNode;
  gateway: GatewayConnection;
  onDeleted: () => void;
  onRenamed: (gatewayName: string) => void;
}) {
  return (
    <Flex
      alignItems={{ default: "alignItemsFlexStart" }}
      flexWrap={{ default: "wrap" }}
      justifyContent={{ default: "justifyContentSpaceBetween" }}
    >
      <FlexItem>
        <Flex alignItems={{ default: "alignItemsCenter" }}>
          <FlexItem>
            <Title headingLevel="h1" size="2xl">
              {gateway.name}
            </Title>
          </FlexItem>
          <FlexItem>
            <GatewayStatus status={gateway.status} />
          </FlexItem>
        </Flex>
        <Content>{description && <p>{description}</p>}</Content>
      </FlexItem>
      <FlexItem>
        <GatewayDetailActions
          gateway={gateway}
          onDeleted={onDeleted}
          onRenamed={onRenamed}
        />
      </FlexItem>
    </Flex>
  );
}
