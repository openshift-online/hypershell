import {
  ClipboardCopyButton,
  CodeBlock,
  CodeBlockAction,
  CodeBlockCode,
  Content,
  Skeleton,
  Title,
} from "@patternfly/react-core";
import { type ReactNode, useEffect, useId, useState } from "react";
import { useIntl } from "react-intl";

import { messages } from "../messages";
import { highlightCommand } from "./command-highlight";
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
  const [highlighted, setHighlighted] = useState<string>();

  // Highlight asynchronously so the plain command renders immediately and the
  // themed markup swaps in once Shiki resolves. Copy always uses the raw
  // `command`, so highlighting never changes what lands on the clipboard.
  useEffect(() => {
    let active = true;
    void highlightCommand(command).then((html) => {
      if (active) {
        setHighlighted(html);
      }
    });
    return () => {
      active = false;
    };
  }, [command]);

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
      {highlighted ? (
        <div
          className={styles.highlighted}
          // Shiki output is derived from the trusted `command` string above;
          // it is display-only markup, so injecting it as HTML is safe.
          dangerouslySetInnerHTML={{ __html: highlighted }}
        />
      ) : (
        <CodeBlockCode id={id}>{command}</CodeBlockCode>
      )}
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
