import {
  Drawer,
  DrawerContent,
  DrawerPanelContent,
} from "@patternfly/react-core";
import {
  useId,
  useRef,
  useState,
  type CSSProperties,
  type KeyboardEvent,
  type ReactNode,
} from "react";
import { useIntl } from "react-intl";
import { useLocation } from "react-router";

import { currentPathAndQuery, signInHref } from "../../lib/sign-in";
import { useTerminalInternals, useTerminalSnapshot } from "./terminal-context";
import { terminalMessages } from "./terminal-messages";
import { TerminalPanel } from "./terminal-panel";
import { useBeforeUnloadGuard } from "./use-before-unload-guard";
import { useTerminalLayout } from "./use-terminal-layout";
import styles from "./terminal.module.css";

/*
 * Specialized-widget admission (WEB-UIEX-01, UIS-PFLY-06): see
 * components/web-console/docs/decisions/UIS-PFLY-06-hsctl-terminal.md for the
 * decision, accessibility review, and bundle assessment.
 */

/**
 * Wraps the console page in a PatternFly inline Drawer (UIS-PFLY-05) that
 * docks the hsctl terminal at the bottom or the right, so the page reflows
 * beside it instead of being covered (WEB-TERM-02). While closed, the panel
 * renders nothing.
 */
export function TerminalDrawer({ children }: { children: ReactNode }) {
  const intl = useIntl();
  const location = useLocation();
  const snapshot = useTerminalSnapshot();
  const { api, panelId, preferences, store } = useTerminalInternals();
  const layout = useTerminalLayout(preferences);
  const titleId = useId();
  const initialFocusRef = useRef<HTMLElement | null>(null);
  const [input, setInput] = useState("");
  const [inputFocused, setInputFocused] = useState(false);
  const { isOpen } = snapshot;

  useBeforeUnloadGuard(
    isOpen && (inputFocused || input.length > 0 || snapshot.running !== null),
  );

  const onPanelKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === "Escape" && !event.defaultPrevented) {
      event.preventDefault();
      api.close();
    }
  };

  const panelStyle = {
    "--pf-v6-c-drawer__panel--FlexBasis": `${String(layout.size)}px`,
  } as CSSProperties;

  return (
    <Drawer
      className={styles.drawer}
      isExpanded={isOpen}
      isInline
      position={layout.dock === "right" ? "end" : "bottom"}
    >
      <DrawerContent
        panelContent={
          <DrawerPanelContent
            className={styles.panelContent}
            data-terminal-panel={panelId}
            id={panelId}
            onKeyDown={onPanelKeyDown}
            style={panelStyle}
            {...(isOpen
              ? {
                  "aria-label": intl.formatMessage(
                    terminalMessages.regionLabel,
                  ),
                  role: "region",
                }
              : {})}
          >
            {isOpen ? (
              <TerminalPanel
                announcement={snapshot.announcement}
                bounds={layout.bounds}
                canDockRight={layout.canDockRight}
                commandHistory={snapshot.commandHistory}
                controlsId={panelId}
                dock={layout.dock}
                focusOnMount
                history={snapshot.history}
                initialFocusRef={initialFocusRef}
                input={input}
                isRunning={snapshot.running !== null}
                onCancel={store.cancelRunning}
                onClear={store.clear}
                onClose={api.close}
                onDockChange={layout.setDock}
                onInputChange={setInput}
                onInputFocusChange={setInputFocused}
                onInterrupt={store.interrupt}
                onResize={layout.resize}
                onRetry={store.retryLoad}
                onSubmit={store.submit}
                runtime={snapshot.runtime}
                scrollRequest={snapshot.scrollRequest}
                sessionStatus={snapshot.session.status}
                signInHref={signInHref(currentPathAndQuery(location))}
                size={layout.size}
                titleId={titleId}
              />
            ) : null}
          </DrawerPanelContent>
        }
      >
        {children}
      </DrawerContent>
    </Drawer>
  );
}
