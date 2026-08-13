import {
  ClipboardCopy,
  Content,
  ExpandableSection,
  Title,
} from "@patternfly/react-core";
import { type ReactNode, useState } from "react";
import { useIntl } from "react-intl";

import { messages } from "../messages";
import {
  buildInferenceSetCommand,
  buildProviderCreateCommand,
  buildProviderFromExistingCommand,
  buildSandboxCreateCommand,
  gcloudAdcLoginCommand,
  inferenceModelPlaceholder,
  sandboxNamePlaceholder,
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

  return (
    <ClipboardCopy
      clickTip={intl.formatMessage(messages.copied)}
      copyAriaLabel={ariaLabel}
      hoverTip={intl.formatMessage(messages.copy)}
      isCode
      isReadOnly
    >
      {command}
    </ClipboardCopy>
  );
}

function ConnectionStep({
  children,
  description,
  title,
}: {
  children: ReactNode;
  description: string;
  title: string;
}) {
  return (
    <li className={styles.step}>
      <Title headingLevel="h2" size="lg">
        {title}
      </Title>
      <Content component="p">{description}</Content>
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
  description: string;
  title: string;
}) {
  return (
    <div className={styles.detail}>
      <Title headingLevel="h3" size="md">
        {title}
      </Title>
      <Content component="p">{description}</Content>
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
      <ConnectionStep
        description={intl.formatMessage(messages.connectionLoginDescription)}
        title={intl.formatMessage(messages.connectionLoginTitle)}
      >
        {loginCommand ? (
          <GatewayCliCopy gateway={gateway} />
        ) : (
          <Content component="p">
            {intl.formatMessage(messages.connectionLoginUnavailable)}
          </Content>
        )}
      </ConnectionStep>

      <ConnectionStep
        description={intl.formatMessage(messages.connectionProviderDescription)}
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
            description={intl.formatMessage(
              messages.connectionProviderAdcDescription,
            )}
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
          <ProviderStepDetail
            ariaLabel={intl.formatMessage(messages.copyInferenceCommand)}
            command={buildInferenceSetCommand()}
            description={intl.formatMessage(
              messages.connectionProviderRoutingDescription,
              { model: inferenceModelPlaceholder },
            )}
            title={intl.formatMessage(messages.connectionProviderRoutingTitle)}
          />
          <Content component="p">
            {intl.formatMessage(messages.connectionProviderCaveat)}
          </Content>
        </ExpandableSection>
      </ConnectionStep>

      <ConnectionStep
        description={intl.formatMessage(messages.connectionSandboxDescription, {
          sandbox: sandboxNamePlaceholder,
        })}
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
