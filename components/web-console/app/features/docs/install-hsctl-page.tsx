import {
  Button,
  Content,
  Stack,
  Tab,
  Tabs,
  TabTitleText,
} from "@patternfly/react-core";
import { TerminalIcon } from "@patternfly/react-icons";
import { externalLinkUrls } from "@openshift-online/hypershell-gateway-management-ui";
import { useMemo } from "react";
import { FormattedMessage, useIntl } from "react-intl";
import { useSearchParams } from "react-router";

import {
  buildInstallHsctlCommand,
  detectInstallOs,
  hsctlWindowsAsset,
  installOsValues,
  parseInstallOs,
  type InstallOs,
} from "../../application/docs/install-hsctl";
import { readBrowserRuntimeConfig } from "../../composition/browser-runtime-config";
import { useTerminal } from "../terminal/terminal-context";
import { DocsArticle } from "./docs-article";
import { DocsCommand } from "./docs-command";
import { useDocsPageViewed } from "./docs-facts";
import { docsMessages } from "./docs-messages";
import styles from "./docs-page.module.css";
import { useDocsSession } from "./use-docs-session";

const osMessages = {
  linux: docsMessages.installLinux,
  macos: docsMessages.installMacos,
} as const;

function browserPlatformHints() {
  if (typeof navigator === "undefined") {
    return {};
  }
  const userAgentData = (
    navigator as Navigator & { userAgentData?: { platform?: string } }
  ).userAgentData;
  return {
    platform: navigator.platform,
    userAgent: navigator.userAgent,
    ...(userAgentData?.platform === undefined
      ? {}
      : { userAgentDataPlatform: userAgentData.platform }),
  };
}

export interface InstallHsctlSurfaceProps {
  readonly downloadBaseUrl?: string;
  readonly onOpenTerminal: () => void;
  readonly onSelectOs: (os: InstallOs) => void;
  readonly selectedOs: InstallOs;
}

/** Install hsctl page without data hooks (WEB-UDOC-08). */
export function InstallHsctlSurface({
  downloadBaseUrl,
  onOpenTerminal,
  onSelectOs,
  selectedOs,
}: InstallHsctlSurfaceProps) {
  const intl = useIntl();
  return (
    <DocsArticle
      subtitle={<FormattedMessage {...docsMessages.installSubtitle} />}
      title={<FormattedMessage {...docsMessages.installTitle} />}
    >
      <div className={styles.section}>
        <Tabs
          activeKey={selectedOs}
          aria-label={intl.formatMessage(docsMessages.installTabs)}
          mountOnEnter
          onSelect={(_event, key) => {
            const os = parseInstallOs(String(key));
            if (os !== undefined) {
              onSelectOs(os);
            }
          }}
        >
          {installOsValues.map((os) => (
            <Tab
              eventKey={os}
              key={os}
              title={
                <TabTitleText>
                  <FormattedMessage {...osMessages[os]} />
                </TabTitleText>
              }
            >
              <Stack className={styles.section} hasGutter>
                <Content component="p">
                  <FormattedMessage {...docsMessages.installCommandBody} />
                </Content>
                <Content component="p">
                  {downloadBaseUrl === undefined ? (
                    <FormattedMessage
                      {...docsMessages.installReleaseChoose}
                      values={{
                        releasesLink: (
                          <a
                            href={externalLinkUrls.hyperShellReleases}
                            rel="noreferrer"
                            target="_blank"
                          >
                            <FormattedMessage
                              {...docsMessages.installReleasesLink}
                            />
                          </a>
                        ),
                      }}
                    />
                  ) : (
                    <FormattedMessage
                      {...docsMessages.installReleaseConfigured}
                    />
                  )}
                </Content>
                <DocsCommand
                  command={buildInstallHsctlCommand(os, downloadBaseUrl)}
                  label={intl.formatMessage(docsMessages.installCommandLabel, {
                    os: intl.formatMessage(osMessages[os]),
                  })}
                />
              </Stack>
            </Tab>
          ))}
        </Tabs>
      </div>
      <Content className={styles.muted} component="p">
        <FormattedMessage
          {...docsMessages.installWindows}
          values={{ asset: <code>{hsctlWindowsAsset}</code> }}
        />
      </Content>
      <Stack className={styles.section} hasGutter>
        <Content component="p">
          <FormattedMessage {...docsMessages.openTerminalBody} />
        </Content>
        <div>
          <Button
            icon={<TerminalIcon aria-hidden="true" />}
            onClick={onOpenTerminal}
            variant="secondary"
          >
            <FormattedMessage {...docsMessages.openTerminal} />
          </Button>
        </div>
      </Stack>
    </DocsArticle>
  );
}

export function InstallHsctlPage() {
  useDocsPageViewed("/docs/cli/install-hsctl", useDocsSession());
  const terminal = useTerminal();
  const [searchParameters, setSearchParameters] = useSearchParams();
  const detectedOs = useMemo(() => detectInstallOs(browserPlatformHints()), []);
  const selectedOs = parseInstallOs(searchParameters.get("os")) ?? detectedOs;
  const downloadBaseUrl = readBrowserRuntimeConfig().hsctlDownloadBaseUrl;

  return (
    <InstallHsctlSurface
      {...(downloadBaseUrl === undefined ? {} : { downloadBaseUrl })}
      onOpenTerminal={() => {
        terminal.open();
      }}
      onSelectOs={(os) => {
        const next = new URLSearchParams(searchParameters);
        next.set("os", os);
        setSearchParameters(next);
      }}
      selectedOs={selectedOs}
    />
  );
}
