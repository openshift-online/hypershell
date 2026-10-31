import { Label } from "@patternfly/react-core";
import { useIntl } from "react-intl";

import type { SpecNode } from "../domain/spec-document";
import { messages } from "../messages";

/**
 * Badges that lead a spec node's title: its ID, in a darker blue for a
 * requirement than for a scenario, or a "Scenario" badge for a scenario
 * that has no ID yet.
 */
export function NodeLabels({ node }: { node: SpecNode }) {
  const intl = useIntl();
  if (node.specId) {
    return (
      <Label
        className={
          node.kind === "spec" ? "sv-id-label--requirement" : undefined
        }
        color="blue"
        isCompact
      >
        {node.specId}
      </Label>
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
