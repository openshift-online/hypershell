import {
  ClipboardCopyButton,
  CodeBlock,
  CodeBlockAction,
  CodeBlockCode,
  Content,
  Skeleton,
  Title,
} from "@patternfly/react-core";
import { type ReactNode, useId, useState } from "react";
import { useIntl } from "react-intl";

import { messages } from "../messages";
import {
  buildSandboxCreateCommand,
  buildSetupScript,
  type GatewayConnection,
} from "./gateway-connections";
import styles from "./gateway-connection-steps.module.css";

function CommandCopy({
  ariaLabel,
  command,
}: {
  ariaLabel: string;
  command: string;
}) {
  const intl = useIntl();
  const id = useId();
  const [copied, setCopied] = useState(false);

  const handleCopy = () => {
    void navigator.clipboard.writeText(command);
    setCopied(true);
  };

  return (
    <CodeBlock
      actions={
        <CodeBlockAction>
          <ClipboardCopyButton
            aria-label={ariaLabel}
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
      <CodeBlockCode id={id}>{command}</CodeBlockCode>
    </CodeBlock>
  );
}

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
  const setupScript = buildSetupScript(gateway);

  return (
    <ol className={styles.steps}>
      <ConnectionStep
        description={intl.formatMessage(messages.connectionSetupDescription)}
        title={intl.formatMessage(messages.connectionSetupTitle)}
      >
        {setupScript ? (
          <CommandCopy
            ariaLabel={intl.formatMessage(messages.copySetupCommand)}
            command={setupScript}
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
        <CommandCopy
          ariaLabel={intl.formatMessage(messages.copySandboxCommand)}
          command={buildSandboxCreateCommand()}
        />
      </ConnectionStep>
    </ol>
  );
}
