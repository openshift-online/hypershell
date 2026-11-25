import { Button } from "@patternfly/react-core";
import { TerminalIcon } from "@patternfly/react-icons";
import { useIntl } from "react-intl";

import { useTerminal, useTerminalInternals } from "./terminal-context";
import { terminalMessages } from "./terminal-messages";

/**
 * The quiet masthead icon button that opens and closes the hsctl terminal
 * (WEB-TERM-01). Ctrl+` toggles the same drawer from anywhere.
 */
export function TerminalTrigger() {
  const intl = useIntl();
  const { panelId, triggerRef } = useTerminalInternals();
  const terminal = useTerminal();
  return (
    <Button
      aria-controls={terminal.isOpen ? panelId : undefined}
      aria-expanded={terminal.isOpen}
      aria-keyshortcuts="Control+`"
      aria-label={intl.formatMessage(
        terminal.isOpen
          ? terminalMessages.closeTerminal
          : terminalMessages.openTerminal,
      )}
      icon={<TerminalIcon />}
      onClick={terminal.toggle}
      ref={triggerRef}
      variant="plain"
    />
  );
}
