import { Content, Stack } from "@patternfly/react-core";
import {
  buildGatewaySelectCommand,
  buildOpenShellInstallCommand,
  isGatewayReadyToConnect,
  toGatewayConnection,
  type GatewayRecord,
} from "@openshift-online/hypershell-gateway-management-ui";
import { useCallback, useState } from "react";
import { FormattedMessage, useIntl } from "react-intl";
import { useSearchParams } from "react-router";

import {
  buildConnectPlaceholder,
  buildRunbookProviderCommands,
  buildRunbookSandboxCommand,
  buildVpnCheckCommand,
  gatewayNamePlaceholder,
  runbookCredentialVariables,
} from "../../application/docs/runbook-commands";
import type { RunbookId } from "../../application/docs/runbook-placement";
import { DocsArticle } from "./docs-article";
import { DocsCommand } from "./docs-command";
import { useDocsPageViewed } from "./docs-facts";
import { docsMessages } from "./docs-messages";
import styles from "./docs-page.module.css";
import { RunbookCell } from "./runbook-cell";
import { RunbookGatewayStep } from "./runbook-gateway-step";
import { useDocsSession, type DocsSessionState } from "./use-docs-session";

/**
 * Steps 2 and 3 of a runbook. They read only the gateway that Step 1 produced
 * and issue no HyperShell API request (WEB-UIEX-05). Until Step 1 supplies a
 * running gateway, they show clearly marked placeholders that name Step 1.
 */
export function RunbookLocalSteps({
  gateway,
  runbook,
}: {
  gateway: GatewayRecord | undefined;
  runbook: RunbookId;
}) {
  const intl = useIntl();
  const isVpn = runbook === "gitlab-jira";
  const connection = gateway ? toGatewayConnection(gateway) : undefined;
  const ready = connection !== undefined && isGatewayReadyToConnect(connection);
  const installCommand = connection
    ? buildOpenShellInstallCommand(connection)
    : undefined;
  const selectCommand = connection
    ? buildGatewaySelectCommand(connection)
    : undefined;
  const gatewayName = ready ? gateway?.name : undefined;

  return (
    <>
      <RunbookCell
        description={
          isVpn ? (
            <FormattedMessage {...docsMessages.vpnCheckBody} />
          ) : (
            <FormattedMessage {...docsMessages.stepConnectBody} />
          )
        }
        mode="local"
        stepNumber={2}
        title={
          <FormattedMessage
            {...(isVpn
              ? docsMessages.stepVpnConnectTitle
              : docsMessages.stepConnectTitle)}
          />
        }
      >
        <Stack hasGutter>
          {isVpn ? (
            <DocsCommand
              command={buildVpnCheckCommand()}
              label={intl.formatMessage(docsMessages.vpnCheckLabel)}
            />
          ) : null}
          {isVpn ? (
            <Content component="p">
              <FormattedMessage {...docsMessages.stepConnectBody} />
            </Content>
          ) : null}
          {ready && selectCommand ? (
            <>
              {installCommand ? (
                <DocsCommand
                  command={installCommand}
                  label={intl.formatMessage(docsMessages.installCliLabel)}
                />
              ) : null}
              <DocsCommand
                command={selectCommand}
                label={intl.formatMessage(docsMessages.selectGatewayLabel)}
              />
            </>
          ) : (
            <>
              <Content component="p">
                <FormattedMessage {...docsMessages.stepConnectPending} />
              </Content>
              <DocsCommand
                command={buildConnectPlaceholder()}
                label={intl.formatMessage(docsMessages.selectGatewayLabel)}
              />
            </>
          )}
        </Stack>
      </RunbookCell>
      <RunbookCell
        description={
          <FormattedMessage
            {...docsMessages.stepSandboxBody}
            values={{
              variables: runbookCredentialVariables(runbook).join(", "),
            }}
          />
        }
        mode="local"
        stepNumber={3}
        title={<FormattedMessage {...docsMessages.stepSandboxTitle} />}
      >
        <Stack hasGutter>
          {gatewayName === undefined ? (
            <Content component="p">
              <FormattedMessage
                {...docsMessages.stepSandboxPlaceholder}
                values={{ placeholder: gatewayNamePlaceholder }}
              />
            </Content>
          ) : null}
          <DocsCommand
            command={buildRunbookProviderCommands(runbook, gatewayName)}
            label={intl.formatMessage(docsMessages.providersLabel)}
          />
          <DocsCommand
            command={buildRunbookSandboxCommand(runbook, gatewayName)}
            label={intl.formatMessage(docsMessages.sandboxLabel)}
          />
        </Stack>
      </RunbookCell>
    </>
  );
}

/** A runbook page without data hooks, for stories and tests. */
export function RunbookPageSurface({
  runbook,
  step1,
  gateway,
}: {
  gateway: GatewayRecord | undefined;
  runbook: RunbookId;
  step1: React.ReactNode;
}) {
  const intl = useIntl();
  const isVpn = runbook === "gitlab-jira";
  return (
    <DocsArticle
      subtitle={
        <FormattedMessage
          {...(isVpn
            ? docsMessages.gitlabRunbookSubtitle
            : docsMessages.githubRunbookSubtitle)}
        />
      }
      title={
        <FormattedMessage
          {...(isVpn
            ? docsMessages.gitlabRunbookTitle
            : docsMessages.githubRunbookTitle)}
        />
      }
    >
      <ol
        aria-label={intl.formatMessage(docsMessages.runbookSteps)}
        className={styles.steps}
      >
        {step1}
        <RunbookLocalSteps gateway={gateway} runbook={runbook} />
      </ol>
    </DocsArticle>
  );
}

/** Reads the optional `name` prefill; it never triggers a request. */
function usePrefillName(): string | undefined {
  const [searchParameters] = useSearchParams();
  return searchParameters.get("name") ?? undefined;
}

export function RunbookPage({
  runbook,
  session: sessionOverride,
}: {
  runbook: RunbookId;
  session?: DocsSessionState;
}) {
  const detectedSession = useDocsSession();
  const session = sessionOverride ?? detectedSession;
  const prefillName = usePrefillName();
  const [gateway, setGateway] = useState<GatewayRecord>();
  const onGatewayChange = useCallback((next: GatewayRecord | undefined) => {
    setGateway(next);
  }, []);
  useDocsPageViewed(
    runbook === "github-jira"
      ? "/docs/runbooks/github-jira"
      : "/docs/runbooks/gitlab-jira",
    session,
  );

  return (
    <RunbookPageSurface
      gateway={gateway}
      runbook={runbook}
      step1={
        <RunbookGatewayStep
          onGatewayChange={onGatewayChange}
          {...(prefillName === undefined ? {} : { prefillName })}
          runbook={runbook}
          session={session}
        />
      }
    />
  );
}
