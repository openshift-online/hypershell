import {
  Alert,
  Button,
  Content,
  Skeleton,
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
  buildSandboxCreateCommand,
  claudeModel,
  type GatewayConnection,
  installDocsUrl,
  sandboxName as defaultSandboxName,
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
const setupMarkers = [providerMarker, modelMarker];
const sandboxMarkers = [sandboxMarker, modelMarker];

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
}: {
  gateway: GatewayConnection;
}) {
  const intl = useIntl();
  const [providerName, setProviderName] = useState(vertexProviderName);
  const [model, setModel] = useState(claudeModel);
  const [sandboxName, setSandboxName] = useState(defaultSandboxName);

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
        {setupTemplate && setupCopy ? (
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

      <ConnectionStep
        description={intl.formatMessage(messages.connectionSandboxDescription)}
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
    </ol>
  );
}
