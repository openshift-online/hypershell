import { Button, Content, Flex, FlexItem, Title } from "@patternfly/react-core";
import { useMemo } from "react";
import { useIntl } from "react-intl";

import type { SpecCatalog } from "../domain/spec-document";
import { buildSpecTree } from "../domain/spec-tree";
import { messages } from "../messages";
import { SpecTreeView } from "./SpecTreeView";
import type { Pins } from "./use-pins";

interface PinnedPanelProps {
  catalog: SpecCatalog;
  pins: Pins;
  selectedNodeId: string | undefined;
  onSelect: (nodeId: string) => void;
}

export function PinnedPanel({
  catalog,
  pins,
  selectedNodeId,
  onSelect,
}: PinnedPanelProps) {
  const intl = useIntl();
  // Pins whose heading was renamed or removed no longer resolve; they stay
  // stored so they come back if the heading is restored, and are not shown.
  const tree = useMemo(
    () =>
      buildSpecTree(
        catalog,
        pins.ids.map((nodeId) => ({ nodeId })),
      ).files,
    [catalog, pins.ids],
  );

  return (
    <div className="sv-pinned">
      <Flex
        alignItems={{ default: "alignItemsCenter" }}
        className="sv-pinned__header"
        justifyContent={{ default: "justifyContentSpaceBetween" }}
      >
        <FlexItem>
          <Title headingLevel="h2" size="md">
            {intl.formatMessage(messages.starredTitle)}
          </Title>
        </FlexItem>
        <FlexItem>
          <Button
            isDisabled={pins.ids.length === 0}
            isInline
            onClick={pins.clear}
            variant="link"
          >
            {intl.formatMessage(messages.clearStars)}
          </Button>
        </FlexItem>
      </Flex>
      {tree.length === 0 ? (
        <Content className="sv-pinned__empty" component="p">
          {intl.formatMessage(messages.starredEmpty)}
        </Content>
      ) : (
        <SpecTreeView
          files={tree}
          idPrefix="starred"
          label={intl.formatMessage(messages.starredLabel)}
          onSelect={onSelect}
          onUnstar={pins.unpin}
          selectedNodeId={selectedNodeId}
        />
      )}
    </div>
  );
}
