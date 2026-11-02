import { Label, Tooltip } from "@patternfly/react-core";
import { useIntl } from "react-intl";

import type { SpecNode } from "../domain/spec-document";
import { messages } from "../messages";

interface NodeLabelsProps {
  node: SpecNode;
  /** When set, the ID label is a button that finds the ID's references. */
  onClick?: () => void;
}

/**
 * Badges that lead a spec node's title: its ID, in a darker blue for a
 * requirement than for a scenario, or a "Scenario" badge for a scenario
 * that has no ID yet.
 */
export function NodeLabels({ node, onClick }: NodeLabelsProps) {
  const intl = useIntl();
  if (node.specId) {
    const label = (
      <Label
        className={
          node.kind === "spec" ? "sv-id-label--requirement" : undefined
        }
        color="blue"
        isCompact
        {...(onClick ? { onClick } : {})}
      >
        {node.specId}
      </Label>
    );
    return onClick ? (
      <Tooltip content={intl.formatMessage(messages.findReferences)}>
        {label}
      </Tooltip>
    ) : (
      label
    );
  }
  if (node.kind === "scenario") {
    return (
      <Label color="purple" isCompact variant="outline">
        {intl.formatMessage(messages.scenarioLabel)}
      </Label>
    );
  }
  return null;
}
