import {
  Card,
  CardBody,
  CardTitle,
  ClipboardCopy,
  ClipboardCopyVariant,
  Content,
  Stack,
  Title,
} from "@patternfly/react-core";
import { CheckCircleIcon, TimesCircleIcon } from "@patternfly/react-icons";
import { externalLinkUrls } from "@openshift-online/hypershell-gateway-management-ui";
import { FormattedMessage, useIntl } from "react-intl";
import { useSearchParams } from "react-router";

import {
  readRefusalContext,
  type RefusalContext,
} from "../../application/docs/refusal-context";
import {
  buildDefaultResourcesSandboxCommand,
  buildStandardResourcesSandboxCommand,
  standardSandboxResources,
} from "../../application/docs/runbook-commands";
import { DocsArticle } from "./docs-article";
import { DocsCommand } from "./docs-command";
import { useDocsPageViewed } from "./docs-facts";
import { docsMessages } from "./docs-messages";
import styles from "./docs-page.module.css";
import { RefusalDiagram } from "./refusal-diagram";
import { useDocsSession } from "./use-docs-session";

const blockedExample = [
  "openshell sandbox create \\",
  "  --name my-sandbox \\",
  `  --driver-config-json '{"kubernetes":{"containers":{"agent":{"resources":{"requests":{"cpu":"8","memory":"32Gi"},"limits":{"cpu":"8","memory":"32Gi"}}}}}}'`,
].join("\n");

/**
 * Direct-link help page for a sandbox request refused because of its CPU or
 * memory values (WEB-UDOC-09). Query values appear only after validation and
 * only as text.
 */
export function ResourceLimitsSurface({
  context,
}: {
  context: RefusalContext;
}) {
  const intl = useIntl();
  const policy =
    context.policy ?? intl.formatMessage(docsMessages.refusalPolicyPlaceholder);
  const gateway =
    context.gateway ??
    intl.formatMessage(docsMessages.refusalGatewayPlaceholder);
  const exceptionMessage = intl.formatMessage(
    docsMessages.refusalExceptionMessage,
    { gateway, policy },
  );
  const hasContext =
    context.policy !== undefined || context.gateway !== undefined;

  return (
    <DocsArticle
      metadata={
        hasContext ? (
          <div
            aria-label={intl.formatMessage(docsMessages.refusalContextLabel)}
            className={styles.metadata}
            role="group"
          >
            {context.policy === undefined ? null : (
              <span>
                <FormattedMessage
                  {...docsMessages.refusalPolicy}
                  values={{ policy: context.policy }}
                />
              </span>
            )}
            {context.gateway === undefined ? null : (
              <span>
                <FormattedMessage
                  {...docsMessages.refusalGateway}
                  values={{ gateway: context.gateway }}
                />
              </span>
            )}
          </div>
        ) : undefined
      }
      subtitle={<FormattedMessage {...docsMessages.refusalStatement} />}
      title={<FormattedMessage {...docsMessages.refusalTitle} />}
    >
      <Stack className={styles.section} hasGutter>
        <Title headingLevel="h2" size="xl">
          <FormattedMessage {...docsMessages.refusalFixTitle} />
        </Title>
        <Content component="p">
          <FormattedMessage {...docsMessages.refusalDefaultFixBody} />
        </Content>
        <DocsCommand
          command={buildDefaultResourcesSandboxCommand()}
          label={intl.formatMessage(docsMessages.refusalDefaultFixLabel)}
        />
        <Content component="p">
          <FormattedMessage {...docsMessages.refusalStandardFixBody} />
        </Content>
        <DocsCommand
          command={buildStandardResourcesSandboxCommand()}
          label={intl.formatMessage(docsMessages.refusalStandardFixLabel)}
        />
        <Content component="p">
          <FormattedMessage
            {...docsMessages.refusalStandardValues}
            values={{
              cpuLimit: <code>{standardSandboxResources.limits.cpu}</code>,
              cpuRequest: <code>{standardSandboxResources.requests.cpu}</code>,
              memoryLimit: (
                <code>{standardSandboxResources.limits.memory}</code>
              ),
              memoryRequest: (
                <code>{standardSandboxResources.requests.memory}</code>
              ),
            }}
          />
        </Content>
      </Stack>
      <Stack className={styles.section} hasGutter>
        <Title headingLevel="h2" size="xl">
          <FormattedMessage {...docsMessages.refusalCompareTitle} />
        </Title>
        <div className={styles.compare}>
          <Card component="div">
            <CardTitle>
              <span className={styles.iconLink}>
                <TimesCircleIcon aria-hidden="true" />
                <FormattedMessage {...docsMessages.refusalBlocked} />
              </span>
            </CardTitle>
            <CardBody>
              <Stack hasGutter>
                <Content component="p">
                  <FormattedMessage {...docsMessages.refusalBlockedBody} />
                </Content>
                <DocsCommand
                  command={blockedExample}
                  label={intl.formatMessage(docsMessages.refusalBlocked)}
                />
              </Stack>
            </CardBody>
          </Card>
          <Card component="div">
            <CardTitle>
              <span className={styles.iconLink}>
                <CheckCircleIcon aria-hidden="true" />
                <FormattedMessage {...docsMessages.refusalAllowed} />
              </span>
            </CardTitle>
            <CardBody>
              <Stack hasGutter>
                <Content component="p">
                  <FormattedMessage {...docsMessages.refusalAllowedBody} />
                </Content>
                <DocsCommand
                  command={buildDefaultResourcesSandboxCommand()}
                  label={intl.formatMessage(docsMessages.refusalAllowed)}
                />
              </Stack>
            </CardBody>
          </Card>
        </div>
      </Stack>
      <Stack className={styles.section} hasGutter>
        <Title headingLevel="h2" size="xl">
          <FormattedMessage {...docsMessages.refusalExceptionTitle} />
        </Title>
        <Content component="p">
          <FormattedMessage
            {...docsMessages.refusalExceptionBody}
            values={{
              supportLink: (
                <a
                  href={externalLinkUrls.supportSlackChannel}
                  rel="noreferrer"
                  target="_blank"
                >
                  <FormattedMessage {...docsMessages.refusalSupportLink} />
                </a>
              ),
            }}
          />
        </Content>
        <ClipboardCopy
          clickTip={intl.formatMessage(docsMessages.copied)}
          hoverTip={intl.formatMessage(docsMessages.refusalExceptionCopy)}
          isExpanded
          isReadOnly
          textAriaLabel={intl.formatMessage(docsMessages.refusalExceptionCopy)}
          toggleAriaLabel={intl.formatMessage(
            docsMessages.refusalExceptionCopy,
          )}
          variant={ClipboardCopyVariant.expansion}
        >
          {exceptionMessage}
        </ClipboardCopy>
      </Stack>
      <div className={styles.section}>
        <RefusalDiagram />
      </div>
    </DocsArticle>
  );
}

export function ResourceLimitsPage() {
  useDocsPageViewed("/docs/refusals/resource-limits", useDocsSession());
  const [searchParameters] = useSearchParams();
  return (
    <ResourceLimitsSurface context={readRefusalContext(searchParameters)} />
  );
}
