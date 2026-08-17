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
  buildProviderCreateCommand,
  buildSandboxCreateCommand,
  buildSandboxPolicyCommand,
  isGatewayReadyToConnect,
  type GatewayConnection,
} from "./gateway-connections";
import { GatewayCliCopy } from "./gateway-detail-header";
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
  const readyToConnect = isGatewayReadyToConnect(gateway);

  return (
    <ol className={styles.steps}>
      <ConnectionStep title={intl.formatMessage(messages.connectionLoginTitle)}>
        {readyToConnect ? (
          <GatewayCliCopy gateway={gateway} />
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
        title={intl.formatMessage(messages.connectionProviderTitle)}
      >
        <CommandCopy
          ariaLabel={intl.formatMessage(messages.copyProviderCommand)}
          command={buildProviderCreateCommand()}
        />
      </ConnectionStep>

      <ConnectionStep
        description={intl.formatMessage(messages.connectionPolicyDescription)}
        title={intl.formatMessage(messages.connectionPolicyTitle)}
      >
        <CommandCopy
          ariaLabel={intl.formatMessage(messages.copyPolicyCommand)}
          command={buildSandboxPolicyCommand()}
        />
      </ConnectionStep>

      <ConnectionStep
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
