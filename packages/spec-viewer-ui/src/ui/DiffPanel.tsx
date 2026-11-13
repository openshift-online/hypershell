import { Button, Label, Tooltip } from "@patternfly/react-core";
import { FolderIcon } from "@patternfly/react-icons";
import {
  Table,
  Tbody,
  Td,
  Th,
  Thead,
  TreeRowWrapper,
  Tr,
  type TdProps,
} from "@patternfly/react-table";
import { useState } from "react";
import { useIntl, type IntlShape } from "react-intl";

import type { DiffEntry, DiffStatus, SpecDiff } from "../domain/spec-diff";
import { messages } from "../messages";
import { NodeLabels } from "./NodeLabels";
import { nodeTitle } from "./NodeText";

interface DiffPanelProps {
  diff: SpecDiff;
  /** Key of the open entry. */
  selectedKey: string | undefined;
  onSelect: (key: string) => void;
}

/** Label text and color of each change. Unchanged entries have no label. */
export function changeLabel(
  intl: IntlShape,
  status: DiffStatus,
): { text: string; color: "green" | "red" | "orange" } | null {
  switch (status) {
    case "added":
      return { text: intl.formatMessage(messages.diffNew), color: "green" };
    case "removed":
      return { text: intl.formatMessage(messages.diffRemoved), color: "red" };
    case "changed":
      return {
        text: intl.formatMessage(messages.diffChanged),
        color: "orange",
      };
    case "unchanged":
      return null;
  }
}

export function diffEntryTitle(intl: IntlShape, entry: DiffEntry): string {
  return entry.kind === "item" && entry.node
    ? nodeTitle(intl, entry.node)
    : entry.title;
}

/**
 * The tree of what differs between two branches: folders, files, and the
 * requirements and scenarios inside them. Everything starts expanded, as the
 * tree holds only changes.
 */
export function DiffPanel({ diff, selectedKey, onSelect }: DiffPanelProps) {
  const intl = useIntl();
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());

  const toggle = (key: string) => {
    setCollapsed((current) => {
      const next = new Set(current);
      if (!next.delete(key)) next.add(key);
      return next;
    });
  };

  const nameCell = (entry: DiffEntry): React.ReactNode => {
    // A folder has nothing to open.
    const title =
      entry.kind === "folder" ? (
        <span>{entry.title}</span>
      ) : (
        <Button
          className="sv-conformance__open"
          isInline
          onClick={() => {
            onSelect(entry.key);
          }}
          variant="link"
        >
          {diffEntryTitle(intl, entry)}
        </Button>
      );
    return (
      <span className="sv-conformance__name">
        {entry.kind === "folder" ? <FolderIcon /> : null}
        {entry.node ? <NodeLabels node={entry.node} /> : null}
        <span className="sv-conformance__title">
          {entry.kind === "file" ? (
            <Tooltip content={entry.path} position="bottom-start">
              {title}
            </Tooltip>
          ) : (
            title
          )}
        </span>
      </span>
    );
  };

  // The rows to draw, top to bottom. A collapsed entry hides its children.
  interface RowPlan {
    entry: DiffEntry;
    level: number;
    index: number;
  }
  const plan = (list: DiffEntry[], level: number): RowPlan[] =>
    list.flatMap((entry, index) => [
      { entry, level, index },
      ...(collapsed.has(entry.key) ? [] : plan(entry.children, level + 1)),
    ]);

  // PatternFly names the expand toggles after the row position.
  const rows = plan(diff.entries, 1).map(
    ({ entry, level, index }, position) => {
      const treeRow: TdProps["treeRow"] = {
        rowIndex: position + 1,
        onCollapse: () => {
          toggle(entry.key);
        },
        props: {
          isExpanded: !collapsed.has(entry.key),
          "aria-level": level,
          "aria-posinset": index + 1,
          "aria-setsize": entry.children.length,
        },
      };
      const label = changeLabel(intl, entry.status);
      return (
        <TreeRowWrapper
          className={
            entry.key === selectedKey
              ? "sv-conformance__row--selected"
              : undefined
          }
          key={entry.key}
          row={{ props: treeRow.props as Record<string, unknown> }}
        >
          <Td
            dataLabel={intl.formatMessage(messages.columnName)}
            treeRow={treeRow}
          >
            {nameCell(entry)}
          </Td>
          <Td dataLabel={intl.formatMessage(messages.columnChange)}>
            {label ? (
              <Label color={label.color} isCompact>
                {label.text}
              </Label>
            ) : null}
          </Td>
        </TreeRowWrapper>
      );
    },
  );

  return (
    <div className="sv-results sv-diff-panel">
      <div className="sv-results__body">
        <Table
          aria-label={intl.formatMessage(messages.diffTableLabel)}
          isTreeTable
          variant="compact"
        >
          <Thead>
            <Tr>
              <Th>{intl.formatMessage(messages.columnName)}</Th>
              <Th modifier="fitContent">
                {intl.formatMessage(messages.columnChange)}
              </Th>
            </Tr>
          </Thead>
          <Tbody>{rows}</Tbody>
        </Table>
      </div>
    </div>
  );
}
