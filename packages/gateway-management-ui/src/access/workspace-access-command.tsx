import {
  Alert,
  Button,
  ClipboardCopyButton,
  CodeBlock,
  CodeBlockCode,
  Content,
  Modal,
  ModalBody,
  ModalFooter,
  ModalHeader,
  ModalVariant,
  Spinner,
  Stack,
  StackItem,
  Tooltip,
} from "@patternfly/react-core";
import { CopyIcon } from "@patternfly/react-icons";
import { useQuery } from "@tanstack/react-query";
import { useId, useState } from "react";
import { useIntl } from "react-intl";

import type { GatewayAccessGrantRecord } from "../application/gateway-types";
import { useGatewayUi } from "../gateway-ui-provider";
import { messages } from "../messages";
import { buildWorkspaceMembershipCommand } from "../service-accounts/service-account-commands";
import { accessDirectoryQueryKey } from "./access-data";

// The default OpenShell workspace new users are added to (GAM). The admin can
// change WORKSPACE_NAME in the copied command before running it.
const defaultWorkspace = "default";

function CommandBlock({ command }: { command: string }) {
  const intl = useIntl();
  const id = useId();
  const [copied, setCopied] = useState(false);
  // Overlay the copy button on the code's top-right corner instead of using
  // CodeBlock's `actions`, which renders a separate (near-empty) header row.
  return (
    <div style={{ position: "relative" }}>
      <ClipboardCopyButton
        aria-label={intl.formatMessage(messages.copyWorkspaceGrantCommand)}
        id={`${id}-copy`}
        onClick={() => {
          void navigator.clipboard.writeText(command);
          setCopied(true);
        }}
        onTooltipHidden={() => {
          setCopied(false);
        }}
        style={{
          insetBlockStart: "var(--pf-t--global--spacer--xs)",
          insetInlineEnd: "var(--pf-t--global--spacer--xs)",
          position: "absolute",
          zIndex: 1,
        }}
        variant="plain"
      >
        {intl.formatMessage(copied ? messages.copied : messages.copy)}
      </ClipboardCopyButton>
      <CodeBlock>
        <CodeBlockCode id={id}>{command}</CodeBlockCode>
      </CodeBlock>
    </div>
  );
}

// WorkspaceAccessCommand renders the gateway-admin instructions plus the copyable
// `openshell workspace member add` command for a resolved subject. subject is
// undefined while a lookup is pending/failed; the caller passes isPending so this
// can show a spinner rather than the "unavailable" error during the lookup.
export function WorkspaceAccessCommand({
  isPending = false,
  subject,
}: {
  isPending?: boolean;
  subject: string | undefined;
}) {
  const intl = useIntl();
  const command = subject
    ? buildWorkspaceMembershipCommand(subject, defaultWorkspace)
    : undefined;

  if (isPending) {
    return (
      <Spinner
        aria-label={intl.formatMessage(messages.accessWorkspaceLookupPending)}
        size="md"
      />
    );
  }
  if (!command) {
    return (
      <Alert
        isInline
        title={intl.formatMessage(messages.accessWorkspaceSubjectUnavailable)}
        variant="warning"
      />
    );
  }
  return (
    <Stack hasGutter>
      <StackItem>
        <Content component="p">
          {intl.formatMessage(messages.accessWorkspaceGrantIntro)}
        </Content>
      </StackItem>
      <StackItem>
        <CommandBlock command={command} />
      </StackItem>
    </Stack>
  );
}

// resolveSubject looks a username up in the gateway directory and returns the
// matching user's subject. The grant row carries no subject (it is a Keycloak
// projection, not stored), so it is looked up on demand (GAM).
function useSubjectLookup(
  gatewayId: string,
  username: string,
  enabled: boolean,
) {
  const { gateways } = useGatewayUi();
  return useQuery({
    enabled,
    queryFn: async ({ signal }) => {
      const candidates = await gateways.searchGatewayDirectory(
        gatewayId,
        username,
        signal,
      );
      const match = candidates.find(
        (candidate) =>
          candidate.username.toLowerCase() === username.toLowerCase(),
      );
      return match?.subject ?? "";
    },
    queryKey: [...accessDirectoryQueryKey(gatewayId, username), "subject"],
    staleTime: 60_000,
  });
}

// GrantWorkspaceAccessAction is the per-row icon that re-opens the "Grant
// workspace access" command for an existing user, looking the subject up on open.
export function GrantWorkspaceAccessAction({
  gatewayId,
  grant,
}: {
  gatewayId: string;
  grant: GatewayAccessGrantRecord;
}) {
  const intl = useIntl();
  const titleId = useId();
  const [isOpen, setIsOpen] = useState(false);
  const lookup = useSubjectLookup(gatewayId, grant.username, isOpen);
  const label = intl.formatMessage(messages.copyWorkspaceGrantCommand);

  return (
    <>
      <Tooltip content={label}>
        <Button
          aria-label={label}
          icon={<CopyIcon />}
          onClick={() => {
            setIsOpen(true);
          }}
          variant="plain"
        />
      </Tooltip>
      {isOpen ? (
        <Modal
          aria-labelledby={titleId}
          isOpen
          onClose={() => {
            setIsOpen(false);
          }}
          variant={ModalVariant.medium}
        >
          <ModalHeader
            labelId={titleId}
            title={intl.formatMessage(messages.grantWorkspaceAccess)}
          />
          <ModalBody>
            <WorkspaceAccessCommand
              isPending={lookup.isPending}
              subject={lookup.data}
            />
          </ModalBody>
          <ModalFooter>
            <Button
              onClick={() => {
                setIsOpen(false);
              }}
              variant="primary"
            >
              {intl.formatMessage(messages.close)}
            </Button>
          </ModalFooter>
        </Modal>
      ) : null}
    </>
  );
}
