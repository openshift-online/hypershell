import { Button } from "@patternfly/react-core";
import { ExclamationCircleIcon } from "@patternfly/react-icons";
import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
} from "react";
import { FormattedMessage, useIntl } from "react-intl";

import {
  terminalHistoryLineLimit,
  type TerminalHistory,
  type TerminalLine,
  type TerminalNotice,
} from "../../application/terminal/terminal-history";
import { terminalMessages } from "./terminal-messages";
import styles from "./terminal.module.css";

/** Lines added together share a stagger that covers at most 8 lines. */
const maxStaggeredLines = 8;
const staggerStepMilliseconds = 20;
const bottomTolerancePixels = 4;
/** Shell prompt symbol; a symbol, not translatable text. */
const promptSymbol = "$";

function prefersReducedMotion(): boolean {
  return window.matchMedia("(prefers-reduced-motion: reduce)").matches;
}

function scrollLogTo(element: HTMLElement, top: number) {
  if (typeof element.scrollTo === "function") {
    element.scrollTo({
      behavior: prefersReducedMotion() ? "auto" : "smooth",
      top,
    });
  } else {
    element.scrollTop = top;
  }
}

function NoticeText({
  notice,
  signInHref,
}: {
  notice: TerminalNotice;
  signInHref: string;
}) {
  switch (notice.kind) {
    case "cancelled":
      return <FormattedMessage {...terminalMessages.cancelled} />;
    case "exit-code":
      return (
        <FormattedMessage
          {...terminalMessages.exitCode}
          values={{ exitCode: notice.exitCode }}
        />
      );
    case "incomplete-line":
      return <FormattedMessage {...terminalMessages.incompleteLine} />;
    case "interrupted":
      return <FormattedMessage {...terminalMessages.interrupted} />;
    case "not-hsctl":
      return (
        <FormattedMessage
          {...terminalMessages.notHsctl}
          values={{ program: notice.program }}
        />
      );
    case "runtime-error":
      return <FormattedMessage {...terminalMessages.runtimeError} />;
    case "session-expired":
      return (
        <FormattedMessage
          {...terminalMessages.sessionExpired}
          values={{
            link: (chunks: ReactNode) => (
              <Button component="a" href={signInHref} isInline variant="link">
                {chunks}
              </Button>
            ),
          }}
        />
      );
    case "shell-operators":
      return <FormattedMessage {...terminalMessages.shellOperators} />;
  }
}

function LineContent({
  line,
  signInHref,
}: {
  line: TerminalLine;
  signInHref: string;
}) {
  const intl = useIntl();
  if (line.kind === "command") {
    return (
      <>
        <span aria-hidden="true" className={styles.prompt}>
          {promptSymbol}
        </span>
        {line.source === "runbook" ? (
          <span className={styles.runbookTag}>
            {intl.formatMessage(terminalMessages.runbookTag)}
          </span>
        ) : null}
        {line.text}
      </>
    );
  }
  if (line.kind === "notice") {
    return <NoticeText notice={line.notice} signInHref={signInHref} />;
  }
  if (line.stream === "stderr") {
    return (
      <>
        <ExclamationCircleIcon
          aria-hidden="true"
          className={styles.stderrIcon}
        />
        <span className="pf-v6-screen-reader">
          {intl.formatMessage(terminalMessages.stderrMarker)}
        </span>
        {line.text}
      </>
    );
  }
  return line.text;
}

function lineClassName(line: TerminalLine, animate: boolean): string {
  return [
    styles.line,
    line.kind === "command" ? styles.commandLine : "",
    line.kind === "notice" ? styles.notice : "",
    line.kind === "output" && line.stream === "stderr" ? styles.stderr : "",
    animate ? styles.lineEnter : "",
  ]
    .filter(Boolean)
    .join(" ");
}

export interface TerminalLogProps {
  history: TerminalHistory;
  /** Scroll to the first line of this entry when `serial` changes. */
  scrollRequest: { entryId: string; serial: number } | null;
  signInHref: string;
}

/**
 * The terminal output log (WEB-TERM-04/08/12). Every value renders as React
 * text, so output such as markup stays visible characters. Lines added after
 * the log mounts animate once; existing lines keep stable keys. New output
 * scrolls into view only while the log is scrolled to the bottom.
 */
export function TerminalLog({
  history,
  scrollRequest,
  signInHref,
}: TerminalLogProps) {
  const intl = useIntl();
  const logRef = useRef<HTMLDivElement>(null);
  const atBottomRef = useRef(true);
  // Lines present when the log mounts (for example on reopening the drawer)
  // must not animate again.
  const [firstAnimatedSequence] = useState(() => history.nextSequence);
  const lastLineId = history.lines.at(-1)?.id;

  useLayoutEffect(() => {
    const log = logRef.current;
    if (log !== null && atBottomRef.current) {
      scrollLogTo(log, log.scrollHeight);
    }
  }, [lastLineId]);

  useEffect(() => {
    const log = logRef.current;
    if (scrollRequest === null || log === null) {
      return;
    }
    const target = Array.from(
      log.querySelectorAll<HTMLElement>("[data-entry-id]"),
    ).find((element) => element.dataset.entryId === scrollRequest.entryId);
    if (target !== undefined) {
      atBottomRef.current = false;
      scrollLogTo(log, target.offsetTop - log.offsetTop);
    }
  }, [scrollRequest]);

  return (
    <div
      aria-label={intl.formatMessage(terminalMessages.logLabel)}
      aria-live="off"
      className={styles.log}
      onScroll={(event) => {
        const log = event.currentTarget;
        atBottomRef.current =
          log.scrollHeight - log.scrollTop - log.clientHeight <=
          bottomTolerancePixels;
      }}
      ref={logRef}
      role="log"
      // A scrollable log must be reachable by keyboard to scroll it (WCAG
      // 2.1.1); jsx-a11y treats the log role as non-interactive.
      // eslint-disable-next-line jsx-a11y/no-noninteractive-tabindex
      tabIndex={0}
    >
      {history.droppedLineCount > 0 ? (
        <p className={styles.droppedNotice}>
          <FormattedMessage
            {...terminalMessages.droppedLines}
            values={{
              count: history.droppedLineCount,
              limit: terminalHistoryLineLimit,
            }}
          />
        </p>
      ) : null}
      {history.lines.length === 0 ? (
        <p className={styles.emptyLog}>
          <FormattedMessage {...terminalMessages.emptyLog} />
        </p>
      ) : null}
      {history.lines.map((line) => {
        const animate = line.sequence >= firstAnimatedSequence;
        const style = animate
          ? ({
              "--terminal-line-delay": `${String(
                Math.min(line.batchPosition, maxStaggeredLines - 1) *
                  staggerStepMilliseconds,
              )}ms`,
            } as CSSProperties)
          : undefined;
        return (
          <div
            className={lineClassName(line, animate)}
            data-entry-id={line.entryId}
            key={line.id}
            style={style}
          >
            <LineContent line={line} signInHref={signInHref} />
          </div>
        );
      })}
    </div>
  );
}
