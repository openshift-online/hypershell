import {
  Alert,
  AlertActionLink,
  Button,
  Content,
  Skeleton,
  Spinner,
  Title,
} from "@patternfly/react-core";
import { ExternalLinkAltIcon } from "@patternfly/react-icons";
import { type ReactNode, useState } from "react";
import { useIntl } from "react-intl";

import { messages } from "../messages";
import { CommandBlock } from "./editable-command";
import {
  buildOneTimeSetupScript,
  buildOpenShellInstallCommand,
  buildSandboxConnectCommand,
  buildSandboxCreateCommand,
  claudeModel,
  defaultEditor,
  type GatewayConnection,
  installDocsUrl,
  sandboxConnectDocsUrl,
  sandboxName as defaultSandboxName,
  validEditors,
  vertexProviderName,
} from "./gateway-connections";
import styles from "./gateway-connection-steps.module.css";

// Unique, single-token placeholders substituted into the command templates where
// an editable value goes. They are highlighted (tokenizing as ordinary argument
// words) and then swapped for inline editors; letters-only keeps them to one
// bash token and clear of any real command text. The provider marker appears
// twice in the setup script, so both slots mirror the same value.
const providerMarker = "OSPROVIDERNAMEZ";
const modelMarker = "OSMODELNAMEZ";
const sandboxMarker = "OSSANDBOXNAMEZ";
const editorMarker = "OSEDITORNAMEZ";
const setupMarkers = [providerMarker, modelMarker];
const sandboxMarkers = [sandboxMarker, modelMarker];
const sandboxConnectMarkers = [sandboxMarker, editorMarker];

function ConnectionStep({
  children,
  description,
  title,
}: {
  children: ReactNode;
  description?: string;
  title: string;
}) {
  return (
    <li className={styles.step}>
      <Title headingLevel="h2" size="lg">
        {title}
      </Title>
      {description && <Content component="p">{description}</Content>}
      {children}
    </li>
  );
}

export function GatewayConnectionSteps({
  gateway,
  isProvisioning,
}: {
  gateway: GatewayConnection;
  isProvisioning?: boolean;
}) {
  const intl = useIntl();
  const [providerName, setProviderName] = useState(vertexProviderName);
  const [model, setModel] = useState(claudeModel);
  const [sandboxName, setSandboxName] = useState(defaultSandboxName);
  const [editor, setEditor] = useState(defaultEditor);

  // Marker form drives the (stable) highlight; the resolved form drives copy and
  // matches a whole-block text selection exactly.
  const installCommand = buildOpenShellInstallCommand(gateway);
  const setupTemplate = buildOneTimeSetupScript(gateway, {
    model: modelMarker,
    providerName: providerMarker,
  });
  const setupCopy = buildOneTimeSetupScript(gateway, { model, providerName });

  return (
    <ol className={styles.steps}>
      <ConnectionStep
        description={intl.formatMessage(messages.connectionSetupDescription)}
        title={intl.formatMessage(messages.connectionSetupTitle)}
      >
        {installCommand ? (
          <Alert
            actionLinks={
              <Button
                aria-label={intl.formatMessage(
                  messages.connectionInstallLinkNewTab,
                )}
                component="a"
                href={installDocsUrl}
                icon={<ExternalLinkAltIcon aria-hidden />}
                iconPosition="end"
                isInline
                rel="noopener noreferrer"
                size="sm"
                target="_blank"
                variant="link"
              >
                {intl.formatMessage(messages.connectionInstallLink)}
              </Button>
            }
            className={styles.prereqAlert}
            component="h3"
            isInline
            title={intl.formatMessage(messages.connectionInstallPrereqTitle)}
            variant="info"
          >
            <div className={styles.prereqContent}>
              <Content component="p">
                {intl.formatMessage(messages.connectionInstallPrereq)}
              </Content>
              <CommandBlock
                copyAriaLabel={intl.formatMessage(messages.copyInstallCommand)}
                copyText={installCommand}
              />
            </div>
          </Alert>
        ) : null}
        <Title
          headingLevel="h3"
          size="lg"
          style={{ marginTop: "var(--pf-t--global--spacer--lg)" }}
        >
          {intl.formatMessage(messages.connectionLoginConfigureTitle)}
        </Title>
        {isProvisioning ? (
          <div className={styles.commandPending} style={{ maxWidth: "60rem" }}>
            <Content component="p" style={{ textAlign: "center" }}>
              <Spinner isInline size="md" />{" "}
              {intl.formatMessage(messages.connectionWaitingForProvisioning)}
            </Content>
          </div>
        ) : setupTemplate && setupCopy ? (
          <CommandBlock
            copyAriaLabel={intl.formatMessage(messages.copySetupCommand)}
            copyText={setupCopy}
            labels={{
              [modelMarker]: intl.formatMessage(messages.editModel),
              [providerMarker]: intl.formatMessage(messages.editProviderName),
            }}
            markers={setupMarkers}
            onFieldChange={(marker, value) => {
              if (marker === providerMarker) {
                setProviderName(value);
              } else if (marker === modelMarker) {
                setModel(value);
              }
            }}
            templateCommand={setupTemplate}
            values={{ [modelMarker]: model, [providerMarker]: providerName }}
          />
        ) : (
          <div
            aria-label={intl.formatMessage(messages.connectionLoginUnavailable)}
            className={styles.commandPending}
            role="status"
          >
            <Skeleton width="52%" />
            <Skeleton width="38%" />
            <Skeleton width="72%" />
            <Skeleton width="35%" />
            <Skeleton width="58%" />
          </div>
        )}
      </ConnectionStep>

      {!isProvisioning && (
        <>
          <ConnectionStep
            description={intl.formatMessage(
              messages.connectionSandboxDescription,
            )}
            title={intl.formatMessage(messages.connectionSandboxTitle)}
          >
            <CommandBlock
              copyAriaLabel={intl.formatMessage(messages.copySandboxCommand)}
              copyText={buildSandboxCreateCommand(sandboxName, model)}
              labels={{
                [modelMarker]: intl.formatMessage(messages.editModel),
                [sandboxMarker]: intl.formatMessage(messages.editSandboxName),
              }}
              markers={sandboxMarkers}
              onFieldChange={(marker, value) => {
                if (marker === sandboxMarker) {
                  setSandboxName(value);
                } else if (marker === modelMarker) {
                  setModel(value);
                }
              }}
              templateCommand={buildSandboxCreateCommand(
                sandboxMarker,
                modelMarker,
              )}
              values={{ [modelMarker]: model, [sandboxMarker]: sandboxName }}
            />
          </ConnectionStep>

          <ConnectionStep
            description={intl.formatMessage(
              messages.connectionSandboxConnectDescription,
            )}
            title={intl.formatMessage(messages.connectionSandboxConnectTitle)}
          >
            <Alert
              actionLinks={
                <AlertActionLink
                  aria-label={intl.formatMessage(
                    messages.connectionEditorOptionsLinkNewTab,
                  )}
                  component="a"
                  href={sandboxConnectDocsUrl}
                  icon={<ExternalLinkAltIcon aria-hidden />}
                  iconPosition="end"
                  rel="noopener noreferrer"
                  target="_blank"
                >
                  {intl.formatMessage(messages.connectionEditorOptionsLink)}
                </AlertActionLink>
              }
              className={styles.prereqAlert}
              component="h3"
              isInline
              title={intl.formatMessage(
                messages.connectionEditorOptionsTitle,
              )}
              variant="info"
            >
              {intl.formatMessage(messages.connectionEditorOptions, {
                code: (chunks) => <code>{chunks}</code>,
              })}
            </Alert>
            <CommandBlock
              copyAriaLabel={intl.formatMessage(
                messages.copySandboxConnectCommand,
              )}
              copyText={buildSandboxConnectCommand(sandboxName, editor)}
              labels={{
                [editorMarker]: intl.formatMessage(messages.editEditor),
                [sandboxMarker]: intl.formatMessage(
                  messages.editExistingSandboxName,
                ),
              }}
              markers={sandboxConnectMarkers}
              onFieldChange={(marker, value) => {
                if (marker === sandboxMarker) {
                  setSandboxName(value);
                } else if (marker === editorMarker) {
                  setEditor(value);
                }
              }}
              selectOptions={{ [editorMarker]: validEditors }}
              templateCommand={buildSandboxConnectCommand(
                sandboxMarker,
                editorMarker,
              )}
              values={{ [editorMarker]: editor, [sandboxMarker]: sandboxName }}
            />
          </ConnectionStep>
        </>
      )}
    </ol>
  );
}
