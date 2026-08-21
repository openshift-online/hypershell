import {
  Alert,
  Button,
  Checkbox,
  ClipboardCopy,
  ClipboardCopyButton,
  CodeBlock,
  CodeBlockAction,
  CodeBlockCode,
  Content,
  DescriptionList,
  DescriptionListDescription,
  DescriptionListGroup,
  DescriptionListTerm,
  ExpandableSection,
  Flex,
  FlexItem,
  Stack,
  StackItem,
  TextInput,
} from "@patternfly/react-core";
import { EyeIcon, EyeSlashIcon } from "@patternfly/react-icons";
import { useId, useState } from "react";
import { useIntl } from "react-intl";

import type {
  OpenShellGatewayServiceAccountConnection,
  OpenShellGatewayServiceAccountRecord,
} from "../application/gateway-types";
import { messages } from "../messages";
import {
  buildClientCredentialsScript,
  buildOpenShellServiceAccountScript,
  buildWorkspaceMembershipCommand,
} from "./service-account-commands";

function CopyableValue({ label, value }: { label: string; value?: string }) {
  const intl = useIntl();
  return value ? (
    <ClipboardCopy
      clickTip={intl.formatMessage(messages.copied)}
      copyAriaLabel={`${intl.formatMessage(messages.copy)} ${label}`}
      hoverTip={intl.formatMessage(messages.copy)}
      isCode
      isReadOnly
    >
      {value}
    </ClipboardCopy>
  ) : (
    intl.formatMessage(messages.notAvailable)
  );
}

function CopyableCommand({
  copyLabel,
  script,
}: {
  copyLabel: string;
  script?: string;
}) {
  const intl = useIntl();
  const id = useId();
  const [copied, setCopied] = useState(false);
  const [copyFailed, setCopyFailed] = useState(false);

  if (!script) {
    return (
      <Alert
        isInline
        title={intl.formatMessage(messages.serviceAccountCommandsUnavailable)}
        variant="warning"
      />
    );
  }

  const copyScript = async () => {
    try {
      await navigator.clipboard.writeText(script);
      setCopied(true);
      setCopyFailed(false);
    } catch {
      setCopied(false);
      setCopyFailed(true);
    }
  };

  return (
    <Stack hasGutter>
      <StackItem>
        <CodeBlock
          actions={
            <CodeBlockAction>
              <ClipboardCopyButton
                aria-label={copyLabel}
                id={`${id}-copy`}
                onClick={() => {
                  void copyScript();
                }}
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
          <CodeBlockCode id={id}>{script}</CodeBlockCode>
        </CodeBlock>
      </StackItem>
      {copyFailed ? (
        <StackItem>
          <Alert
            isInline
            title={intl.formatMessage(messages.serviceAccountCopyFailed)}
            variant="danger"
          />
        </StackItem>
      ) : null}
    </Stack>
  );
}

export function ServiceAccountSetupView({
  account,
  clientSecret,
  connection,
  isAcknowledged,
  onAcknowledgedChange,
}: {
  account: OpenShellGatewayServiceAccountRecord;
  clientSecret?: string;
  connection: OpenShellGatewayServiceAccountConnection;
  isAcknowledged: boolean;
  onAcknowledgedChange: (value: boolean) => void;
}) {
  const intl = useIntl();
  const [isJwtExpanded, setIsJwtExpanded] = useState(false);
  const [isOpenShellExpanded, setIsOpenShellExpanded] = useState(true);
  const [isSecretVisible, setIsSecretVisible] = useState(false);
  const [secretCopied, setSecretCopied] = useState(false);
  const [secretCopyFailed, setSecretCopyFailed] = useState(false);
  const acknowledgementId = useId();
  const secretId = useId();
  const openShellScript = buildOpenShellServiceAccountScript(
    account.name,
    connection,
  );
  const jwtScript = buildClientCredentialsScript(connection);
  const workspaceMembershipScript =
    account.role === "openshell-user"
      ? buildWorkspaceMembershipCommand(account.subject)
      : undefined;
  const copyClientSecret = async () => {
    if (!clientSecret) {
      return;
    }
    try {
      await navigator.clipboard.writeText(clientSecret);
      setSecretCopied(true);
      setSecretCopyFailed(false);
    } catch {
      setSecretCopied(false);
      setSecretCopyFailed(true);
    }
  };
  return (
    <Stack hasGutter>
      <StackItem>
        <Alert
          isInline
          title={intl.formatMessage(
            clientSecret
              ? messages.serviceAccountSecretOnce
              : messages.serviceAccountSecretUnavailable,
          )}
          variant={clientSecret ? "warning" : "info"}
        />
      </StackItem>
      <StackItem>
        <DescriptionList isHorizontal>
          <DescriptionListGroup>
            <DescriptionListTerm>
              {intl.formatMessage(messages.clientId)}
            </DescriptionListTerm>
            <DescriptionListDescription>
              <CopyableValue
                label={intl.formatMessage(messages.clientId)}
                value={connection.clientId}
              />
            </DescriptionListDescription>
          </DescriptionListGroup>
          {clientSecret ? (
            <DescriptionListGroup>
              <DescriptionListTerm>
                {intl.formatMessage(messages.clientSecret)}
              </DescriptionListTerm>
              <DescriptionListDescription>
                <Stack hasGutter>
                  <StackItem>
                    <Flex flexWrap={{ default: "nowrap" }}>
                      <FlexItem grow={{ default: "grow" }}>
                        <TextInput
                          aria-label={intl.formatMessage(messages.clientSecret)}
                          id={secretId}
                          readOnly
                          type={isSecretVisible ? "text" : "password"}
                          value={clientSecret}
                        />
                      </FlexItem>
                      <FlexItem>
                        <Button
                          aria-label={intl.formatMessage(
                            isSecretVisible
                              ? messages.hideClientSecret
                              : messages.showClientSecret,
                          )}
                          icon={
                            isSecretVisible ? (
                              <EyeSlashIcon aria-hidden />
                            ) : (
                              <EyeIcon aria-hidden />
                            )
                          }
                          onClick={() => {
                            setIsSecretVisible((visible) => !visible);
                          }}
                          variant="control"
                        />
                      </FlexItem>
                      <FlexItem>
                        <Button
                          onClick={() => {
                            void copyClientSecret();
                          }}
                          variant="secondary"
                        >
                          {intl.formatMessage(
                            secretCopied
                              ? messages.clientSecretCopied
                              : messages.copyClientSecret,
                          )}
                        </Button>
                      </FlexItem>
                    </Flex>
                  </StackItem>
                  {secretCopyFailed ? (
                    <StackItem>
                      <Alert
                        isInline
                        title={intl.formatMessage(
                          messages.serviceAccountCopyFailed,
                        )}
                        variant="danger"
                      />
                    </StackItem>
                  ) : null}
                </Stack>
              </DescriptionListDescription>
            </DescriptionListGroup>
          ) : null}
          <DescriptionListGroup>
            <DescriptionListTerm>
              {intl.formatMessage(messages.subject)}
            </DescriptionListTerm>
            <DescriptionListDescription>
              <Stack hasGutter>
                <StackItem>
                  <CopyableValue
                    label={intl.formatMessage(messages.subject)}
                    value={account.subject}
                  />
                </StackItem>
                <StackItem>
                  <Content component="p">
                    {intl.formatMessage(messages.subjectHelp)}
                  </Content>
                </StackItem>
              </Stack>
            </DescriptionListDescription>
          </DescriptionListGroup>
        </DescriptionList>
      </StackItem>
      {account.role === "openshell-user" ? (
        <>
          <StackItem>
            <Alert
              isInline
              title={intl.formatMessage(messages.workspaceMembershipNote)}
              variant="info"
            />
          </StackItem>
          <StackItem>
            <ExpandableSection
              toggleText={intl.formatMessage(messages.grantWorkspaceAccess)}
              toggleWrapper="h3"
            >
              <Stack hasGutter>
                <StackItem>
                  <Content component="p">
                    {intl.formatMessage(messages.workspaceGrantInstructions)}
                  </Content>
                </StackItem>
                <StackItem>
                  <CopyableCommand
                    copyLabel={intl.formatMessage(
                      messages.copyWorkspaceGrantCommand,
                    )}
                    script={workspaceMembershipScript}
                  />
                </StackItem>
              </Stack>
            </ExpandableSection>
          </StackItem>
        </>
      ) : null}
      <StackItem>
        <ExpandableSection
          isExpanded={isOpenShellExpanded}
          onToggle={(_event, expanded) => {
            setIsOpenShellExpanded(expanded);
          }}
          toggleText={intl.formatMessage(messages.openShellCliSetup)}
          toggleWrapper="h3"
        >
          <CopyableCommand
            copyLabel={intl.formatMessage(
              messages.copyOpenShellServiceAccountCommands,
            )}
            script={openShellScript}
          />
        </ExpandableSection>
      </StackItem>
      <StackItem>
        <ExpandableSection
          isExpanded={isJwtExpanded}
          onToggle={(_event, expanded) => {
            setIsJwtExpanded(expanded);
          }}
          toggleText={intl.formatMessage(messages.exchangeCredentialsForJwt)}
          toggleWrapper="h3"
        >
          <CopyableCommand
            copyLabel={intl.formatMessage(messages.copyJwtExchangeCommands)}
            script={jwtScript}
          />
        </ExpandableSection>
      </StackItem>
      {clientSecret ? (
        <StackItem>
          <Checkbox
            id={acknowledgementId}
            isChecked={isAcknowledged}
            label={intl.formatMessage(messages.acknowledgeSecretSaved)}
            onChange={(_event, checked) => {
              onAcknowledgedChange(checked);
            }}
          />
        </StackItem>
      ) : null}
    </Stack>
  );
}
