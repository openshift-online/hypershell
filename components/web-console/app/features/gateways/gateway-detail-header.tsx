import {
  ActionList,
  ActionListItem,
  Button,
  ClipboardCopy,
  Content,
  Flex,
  FlexItem,
  Label,
  Popover,
  Stack,
  StackItem,
  Title,
} from "@patternfly/react-core";
import { ExternalLinkAltIcon } from "@patternfly/react-icons";
import { type ReactNode, useState } from "react";
import { FormattedMessage, useIntl } from "react-intl";

import { messages } from "../../i18n/messages";
import {
  buildGatewayAddCommand,
  gatewayStatusColor,
  type GatewayConnection,
} from "./gateway-connections";
import { GatewayDeleteDialog } from "./gateway-delete-dialog";

export function GatewayCliCopy({ gateway }: { gateway: GatewayConnection }) {
  const intl = useIntl();

  return (
    <ClipboardCopy
      clickTip={intl.formatMessage(messages.copied)}
      copyAriaLabel={intl.formatMessage(messages.copyConnectionCommand, {
        gatewayName: gateway.name,
      })}
      hoverTip={intl.formatMessage(messages.copy)}
      isCode
      isReadOnly
    >
      {buildGatewayAddCommand(gateway)}
    </ClipboardCopy>
  );
}

export function GatewayEndpointCopy({
  gateway,
}: {
  gateway: GatewayConnection;
}) {
  const intl = useIntl();

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
}: {
  gateway: GatewayConnection;
  onDeleted: () => void;
}) {
  const intl = useIntl();
  const [isDeleteOpen, setIsDeleteOpen] = useState(false);

  return (
    <>
      <ActionList>
        <ActionListItem>
          <Button
            onClick={() => {
              setIsDeleteOpen(true);
            }}
            variant="danger"
          >
            <FormattedMessage {...messages.deleteGateway} />
          </Button>
        </ActionListItem>
        <ActionListItem>
          <Popover
            aria-label={intl.formatMessage(messages.connectWithCli)}
            bodyContent={
              <Stack hasGutter>
                <StackItem>
                  <Title headingLevel="h2" size="md">
                    <FormattedMessage {...messages.connectWithCli} />
                  </Title>
                </StackItem>
                <StackItem>
                  <GatewayCliCopy gateway={gateway} />
                </StackItem>
              </Stack>
            }
            closeBtnAriaLabel={intl.formatMessage(messages.close)}
            maxWidth="40rem"
            minWidth="20rem"
            position="bottom-end"
            showClose
            withFocusTrap
          >
            <Button variant="secondary">
              <FormattedMessage {...messages.connectWithCli} />
            </Button>
          </Popover>
        </ActionListItem>
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
      </ActionList>
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
    </>
  );
}

export function GatewayDetailHeader({
  description,
  gateway,
  onDeleted,
}: {
  description: ReactNode;
  gateway: GatewayConnection;
  onDeleted: () => void;
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
            <Label color={gatewayStatusColor(gateway.status)}>
              {gateway.status}
            </Label>
          </FlexItem>
        </Flex>
        <Content>
          <p>{description}</p>
        </Content>
      </FlexItem>
      <FlexItem>
        <GatewayDetailActions gateway={gateway} onDeleted={onDeleted} />
      </FlexItem>
    </Flex>
  );
}
