import { Card, CardBody, CardTitle, Content } from "@patternfly/react-core";
import { ArrowRightIcon } from "@patternfly/react-icons";
import { FormattedMessage } from "react-intl";
import { Link } from "react-router";

import { routePaths } from "../../../shared/routes";
import { DocsArticle } from "./docs-article";
import { useDocsPageViewed } from "./docs-facts";
import { docsMessages } from "./docs-messages";
import styles from "./docs-page.module.css";
import { useDocsSession } from "./use-docs-session";

function TaskCard({
  body,
  title,
  to,
}: {
  body: React.ReactNode;
  title: React.ReactNode;
  to: string;
}) {
  return (
    <Card className={styles.card} component="div" isCompact={false}>
      <CardTitle>
        <Link className={styles.cardLink} to={to}>
          <span>{title}</span>
          <ArrowRightIcon aria-hidden="true" />
        </Link>
      </CardTitle>
      <CardBody>{body}</CardBody>
    </Card>
  );
}

/** Docs home: a title, two task cards, and one muted help line (WEB-UDOC-04). */
export function DocsHomeSurface() {
  return (
    <DocsArticle
      showBackLink={false}
      subtitle={<FormattedMessage {...docsMessages.pageDescriptionHome} />}
      title={<FormattedMessage {...docsMessages.pageTitleHome} />}
    >
      <div className={styles.cards}>
        <TaskCard
          body={<FormattedMessage {...docsMessages.homeGithubCardBody} />}
          title={<FormattedMessage {...docsMessages.homeGithubCardTitle} />}
          to={routePaths.docsRunbookGithubJira}
        />
        <TaskCard
          body={<FormattedMessage {...docsMessages.homeGitlabCardBody} />}
          title={<FormattedMessage {...docsMessages.homeGitlabCardTitle} />}
          to={routePaths.docsRunbookGitlabJira}
        />
      </div>
      <Content className={styles.muted} component="p">
        <FormattedMessage
          {...docsMessages.homeMoreHelp}
          values={{
            installLink: (
              <Link to={routePaths.docsCliInstallHsctl}>
                <FormattedMessage {...docsMessages.homeInstallLink} />
              </Link>
            ),
            limitsLink: (
              <Link to={routePaths.docsRefusalResourceLimits}>
                <FormattedMessage {...docsMessages.homeLimitsLink} />
              </Link>
            ),
          }}
        />
      </Content>
    </DocsArticle>
  );
}

/** The docs home sends no HyperShell API request (WEB-UDOC-04). */
export function DocsHomePage() {
  useDocsPageViewed("/docs", useDocsSession());
  return <DocsHomeSurface />;
}
