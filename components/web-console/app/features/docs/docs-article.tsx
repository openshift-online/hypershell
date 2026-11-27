import { Content, PageSection, Title } from "@patternfly/react-core";
import { ArrowLeftIcon } from "@patternfly/react-icons";
import type { ReactNode } from "react";
import { FormattedMessage } from "react-intl";
import { Link } from "react-router";

import { routePaths } from "../../../shared/routes";
import { docsMessages } from "./docs-messages";
import styles from "./docs-page.module.css";

/**
 * Calm docs page frame: one readable column no wider than 760 CSS pixels with
 * a title and a one-line subtitle (WEB-UDOC-04).
 */
export function DocsArticle({
  children,
  metadata,
  showBackLink = true,
  subtitle,
  title,
}: {
  children: ReactNode;
  metadata?: ReactNode;
  showBackLink?: boolean;
  subtitle: ReactNode;
  title: ReactNode;
}) {
  return (
    <PageSection>
      <article className={styles.column}>
        {showBackLink ? (
          <Content className={styles.backLink} component="p">
            <Link className={styles.iconLink} to={routePaths.docs}>
              <ArrowLeftIcon aria-hidden="true" />
              <FormattedMessage {...docsMessages.backToDocs} />
            </Link>
          </Content>
        ) : null}
        <Title headingLevel="h1" size="2xl">
          {title}
        </Title>
        <Content className={styles.subtitle} component="p">
          {subtitle}
        </Content>
        {metadata}
        {children}
      </article>
    </PageSection>
  );
}
