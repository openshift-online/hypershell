import { Content, Title } from "@patternfly/react-core";
import type { ReactNode } from "react";
import { FormattedMessage, useIntl } from "react-intl";

import { docsMessages } from "./docs-messages";
import styles from "./docs-page.module.css";

export interface RunbookCellOutput {
  readonly failed: boolean;
  readonly lines: readonly string[];
}

/**
 * One notebook-style runbook step (WEB-UDOC-05): a step number, a short title,
 * short prose, where the step runs, its commands, at most one primary action,
 * and, while or after it runs in the browser, a dark read-only output pane
 * that renders text only.
 */
export function RunbookCell({
  actions,
  children,
  description,
  mode,
  output,
  stepNumber,
  title,
}: {
  actions?: ReactNode;
  children?: ReactNode;
  description: ReactNode;
  mode: "browser" | "local";
  output?: RunbookCellOutput;
  stepNumber: number;
  title: ReactNode;
}) {
  const intl = useIntl();
  const headingId = `runbook-step-${String(stepNumber)}`;
  return (
    <li aria-labelledby={headingId} className={styles.cell}>
      <div>
        <div className={styles.stepLabel}>
          <FormattedMessage
            {...docsMessages.stepLabel}
            values={{ number: stepNumber }}
          />
        </div>
        <Title headingLevel="h2" id={headingId} size="xl">
          {title}
        </Title>
        <div className={styles.where}>
          <FormattedMessage
            {...(mode === "browser"
              ? docsMessages.runsInBrowser
              : docsMessages.runsLocally)}
          />
        </div>
      </div>
      <Content component="p">{description}</Content>
      {children}
      {actions ? <div className={styles.actions}>{actions}</div> : null}
      {output && output.lines.length > 0 ? (
        <div
          aria-label={intl.formatMessage(docsMessages.outputLabel, {
            number: stepNumber,
          })}
          role="group"
        >
          <pre className={styles.output}>
            {output.lines.map((line, index) => (
              <span
                className={
                  output.failed && index === output.lines.length - 1
                    ? [styles.outputLine, styles.outputFailed].join(" ")
                    : styles.outputLine
                }
                key={index}
              >
                {line}
              </span>
            ))}
          </pre>
        </div>
      ) : null}
    </li>
  );
}
