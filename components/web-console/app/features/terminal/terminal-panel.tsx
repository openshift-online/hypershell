import {
  Alert,
  AlertActionLink,
  Button,
  DrawerActions,
  DrawerCloseButton,
  DrawerHead,
  Progress,
  ProgressMeasureLocation,
  Spinner,
  Title,
} from "@patternfly/react-core";
import { ArrowsAltHIcon, ArrowsAltVIcon } from "@patternfly/react-icons";
import { useEffect, type RefObject } from "react";
import { FormattedMessage, useIntl } from "react-intl";

import type { TerminalHistory } from "../../application/terminal/terminal-history";
import type { TerminalDock } from "../../application/terminal/terminal-layout";
import { TerminalLog } from "./terminal-log";
import { terminalMessages } from "./terminal-messages";
import { TerminalPrompt } from "./terminal-prompt";
import { TerminalResizeHandle } from "./terminal-resize-handle";
import type {
  TerminalAnnouncement,
  TerminalRuntimeStatus,
  TerminalSessionStatus,
} from "./terminal-store";
import styles from "./terminal.module.css";

export interface TerminalPanelProps {
  announcement: TerminalAnnouncement | null;
  bounds: { max: number; min: number };
  canDockRight: boolean;
  commandHistory: readonly string[];
  controlsId: string;
  dock: TerminalDock;
  /**
   * Move focus to the input, or the sign-in link, when the panel mounts. The
   * drawer sets it because PatternFly mounts panel content after expanding.
   */
  focusOnMount?: boolean;
  history: TerminalHistory;
  /** Receives the element to focus when the drawer opens. */
  initialFocusRef: RefObject<HTMLElement | null>;
  input: string;
  isRunning: boolean;
  onCancel: () => void;
  onClear: () => void;
  onClose: () => void;
  onDockChange: (dock: TerminalDock) => void;
  onInputChange: (value: string) => void;
  onInputFocusChange: (focused: boolean) => void;
  onInterrupt: (line: string) => void;
  onResize: (size: number, commit: boolean) => void;
  onRetry: () => void;
  onSubmit: (line: string) => boolean;
  runtime: TerminalRuntimeStatus;
  scrollRequest: { entryId: string; serial: number } | null;
  sessionStatus: TerminalSessionStatus;
  signInHref: string;
  size: number;
  titleId: string;
}

function RuntimeStatus({
  onRetry,
  runtime,
}: {
  onRetry: () => void;
  runtime: TerminalRuntimeStatus;
}) {
  const intl = useIntl();
  if (runtime.kind === "loading") {
    const progress = runtime.progress;
    if (progress?.totalBytes) {
      return (
        <div className={styles.status}>
          <Progress
            aria-label={intl.formatMessage(terminalMessages.loading)}
            measureLocation={ProgressMeasureLocation.outside}
            max={progress.totalBytes}
            size="sm"
            title={intl.formatMessage(terminalMessages.loading)}
            value={Math.min(progress.loadedBytes, progress.totalBytes)}
            valueText={intl.formatNumber(
              progress.loadedBytes / progress.totalBytes,
              { style: "percent" },
            )}
          />
        </div>
      );
    }
    const label =
      progress && progress.loadedBytes > 0
        ? intl.formatMessage(terminalMessages.loadingBytes, {
            loaded: intl.formatNumber(progress.loadedBytes / 1_000_000, {
              maximumFractionDigits: 1,
              style: "unit",
              unit: "megabyte",
            }),
          })
        : intl.formatMessage(terminalMessages.loading);
    return (
      <div className={styles.status} role="status">
        <Spinner
          aria-hidden="true"
          className={styles.spinner}
          isInline
          size="md"
        />
        {label}
      </div>
    );
  }
  if (runtime.kind === "failed") {
    return (
      <div className={styles.status}>
        <Alert
          actionLinks={
            runtime.failureKind === "load-failed" ? (
              <AlertActionLink onClick={onRetry}>
                {intl.formatMessage(terminalMessages.retry)}
              </AlertActionLink>
            ) : undefined
          }
          isInline
          isPlain
          role="alert"
          title={intl.formatMessage(terminalMessages.loadFailedTitle)}
          variant="danger"
        >
          {intl.formatMessage(
            runtime.failureKind === "unavailable"
              ? terminalMessages.unavailableBody
              : terminalMessages.loadFailedBody,
          )}
        </Alert>
      </div>
    );
  }
  return null;
}

function announcementText(
  announcement: TerminalAnnouncement | null,
  intl: ReturnType<typeof useIntl>,
): string {
  if (announcement === null) {
    return "";
  }
  switch (announcement.kind) {
    case "finished":
      return intl.formatMessage(terminalMessages.commandFinished, {
        exitCode: announcement.exitCode,
      });
    case "cancelled":
      return intl.formatMessage(terminalMessages.commandCancelledStatus);
    case "runtime-error":
      return intl.formatMessage(terminalMessages.commandFailedStatus);
  }
}

/**
 * Presentational content of the open terminal drawer (WEB-TERM-01..12):
 * header with dock and close controls, resize handle, log, and prompt, or
 * only a sign-in link for a signed-out reader. Stories render it with props.
 */
export function TerminalPanel(props: TerminalPanelProps) {
  const intl = useIntl();
  const {
    bounds,
    canDockRight,
    controlsId,
    dock,
    focusOnMount = false,
    initialFocusRef,
    onClose,
    onDockChange,
    onResize,
    sessionStatus,
    signInHref,
    size,
    titleId,
  } = props;

  // Opening moves focus to the terminal input, or to the sign-in link
  // (WEB-TERM-01).
  useEffect(() => {
    if (focusOnMount) {
      initialFocusRef.current?.focus({ preventScroll: true });
    }
  }, [focusOnMount, initialFocusRef, sessionStatus]);

  const header = (
    <DrawerHead>
      <Title headingLevel="h2" id={titleId} size="md">
        {intl.formatMessage(terminalMessages.title)}
      </Title>
      <DrawerActions>
        {sessionStatus === "signed-in" && canDockRight ? (
          <Button
            icon={dock === "bottom" ? <ArrowsAltHIcon /> : <ArrowsAltVIcon />}
            onClick={() => {
              onDockChange(dock === "bottom" ? "right" : "bottom");
            }}
            size="sm"
            variant="link"
          >
            {intl.formatMessage(
              dock === "bottom"
                ? terminalMessages.dockRight
                : terminalMessages.dockBottom,
            )}
          </Button>
        ) : null}
        <DrawerCloseButton
          aria-label={intl.formatMessage(terminalMessages.closePanel)}
          onClick={onClose}
        />
      </DrawerActions>
    </DrawerHead>
  );

  let body;
  if (sessionStatus === "signed-out") {
    body = (
      <div className={styles.signIn}>
        <Button
          component="a"
          href={signInHref}
          isInline
          ref={initialFocusRef as RefObject<HTMLAnchorElement | null>}
          variant="link"
        >
          <FormattedMessage {...terminalMessages.signIn} />
        </Button>
      </div>
    );
  } else if (sessionStatus === "unknown") {
    body = (
      <div className={styles.status} role="status">
        <Spinner
          aria-hidden="true"
          className={styles.spinner}
          isInline
          size="md"
        />
        {intl.formatMessage(terminalMessages.checkingSession)}
      </div>
    );
  } else {
    body = (
      <>
        <TerminalLog
          history={props.history}
          scrollRequest={props.scrollRequest}
          signInHref={signInHref}
        />
        <RuntimeStatus onRetry={props.onRetry} runtime={props.runtime} />
        <div className="pf-v6-screen-reader" role="status">
          {announcementText(props.announcement, intl)}
        </div>
        <TerminalPrompt
          acceptsCommands={props.runtime.kind === "ready"}
          commandHistory={props.commandHistory}
          inputRef={initialFocusRef as RefObject<HTMLInputElement | null>}
          isRunning={props.isRunning}
          onCancel={props.onCancel}
          onChange={props.onInputChange}
          onClear={props.onClear}
          onFocusChange={props.onInputFocusChange}
          onInterrupt={props.onInterrupt}
          onSubmit={props.onSubmit}
          value={props.input}
        />
      </>
    );
  }

  return (
    <div
      className={[styles.panel, dock === "right" ? styles.panelRight : ""].join(
        " ",
      )}
    >
      {sessionStatus === "signed-in" ? (
        <TerminalResizeHandle
          controlsId={controlsId}
          dock={dock}
          max={bounds.max}
          min={bounds.min}
          onResize={onResize}
          size={size}
        />
      ) : null}
      <div className={styles.main}>
        {header}
        {body}
      </div>
    </div>
  );
}
