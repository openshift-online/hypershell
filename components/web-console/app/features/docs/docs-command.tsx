import { CommandBlock } from "@openshift-online/hypershell-gateway-management-ui";
import { useIntl } from "react-intl";

import { docsMessages } from "./docs-messages";
import styles from "./docs-page.module.css";

/**
 * A labeled docs command block. Every docs command uses the shared command
 * block and shell highlighting of the gateway Connection tab, which renders
 * React elements only (WEB-UDOC-10).
 */
export function DocsCommand({
  command,
  label,
}: {
  command: string;
  label: string;
}) {
  const intl = useIntl();
  return (
    <div>
      <div className={styles.commandLabel}>{label}</div>
      <CommandBlock
        copyAriaLabel={intl.formatMessage(docsMessages.copyCommand, { label })}
        copyText={command}
      />
    </div>
  );
}
