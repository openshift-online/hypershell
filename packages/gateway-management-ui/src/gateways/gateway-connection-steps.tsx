import {
  ClipboardCopyButton,
  CodeBlock,
  CodeBlockAction,
  CodeBlockCode,
  Content,
  ExpandableSection,
  Skeleton,
  Title,
} from "@patternfly/react-core";
import { type ReactNode, useId, useState } from "react";
import { useIntl } from "react-intl";

import { messages } from "../messages";
import {
  buildProviderCreateCommand,
  buildProviderFromExistingCommand,
  buildSandboxCreateCommand,
  gcloudAdcLoginCommand,
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

function ProviderStepDetail({
  ariaLabel,
  command,
  description,
  title,
}: {
  ariaLabel: string;
  command: string;
  description?: string;
  title: string;
}) {
  return (
    <div className={styles.detail}>
      <Title headingLevel="h3" size="md">
        {title}
      </Title>
      {description && <Content component="p">{description}</Content>}
      <CommandCopy ariaLabel={ariaLabel} command={command} />
    </div>
  );
}

export function GatewayConnectionSteps({
  gateway,
}: {
  gateway: GatewayConnection;
}) {
  const intl = useIntl();
  const [isDetailsExpanded, setIsDetailsExpanded] = useState(false);
  const loginCommand = gateway.endpoint;

  return (
    <ol className={styles.steps}>
      <ConnectionStep title={intl.formatMessage(messages.connectionLoginTitle)}>
        {loginCommand ? (
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
        <ExpandableSection
          isExpanded={isDetailsExpanded}
          onToggle={(_event, expanded) => {
            setIsDetailsExpanded(expanded);
          }}
          toggleText={intl.formatMessage(
            messages.connectionProviderDetailsToggle,
          )}
        >
          <ProviderStepDetail
            ariaLabel={intl.formatMessage(messages.copyAdcLoginCommand)}
            command={gcloudAdcLoginCommand}
            title={intl.formatMessage(messages.connectionProviderAdcTitle)}
          />
          <ProviderStepDetail
            ariaLabel={intl.formatMessage(
              messages.copyProviderFromExistingCommand,
            )}
            command={buildProviderFromExistingCommand()}
            description={intl.formatMessage(
              messages.connectionProviderFromEnvDescription,
            )}
            title={intl.formatMessage(messages.connectionProviderFromEnvTitle)}
          />
          <Content component="p">
            {intl.formatMessage(messages.connectionProviderCaveat)}
          </Content>
        </ExpandableSection>
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
