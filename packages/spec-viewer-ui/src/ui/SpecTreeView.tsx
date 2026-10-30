import {
  Button,
  TreeView,
  type TreeViewDataItem,
} from "@patternfly/react-core";
import { ThumbtackIcon, TimesIcon } from "@patternfly/react-icons";
import { useIntl, type IntlShape } from "react-intl";

import type { Snippet } from "../domain/search";
import { fileEntryId, type SpecNode } from "../domain/spec-document";
import type { SpecTreeFile, SpecTreeNode } from "../domain/spec-tree";
import { messages } from "../messages";
import { NodeLabels } from "./NodeLabels";

export type TreeAction = "pin" | "unpin";

interface SpecTreeViewProps {
  /** Distinguishes DOM ids when several trees are on the page. */
  idPrefix: string;
  label: string;
  files: SpecTreeFile[];
  selectedNodeId: string | undefined;
  action: TreeAction;
  /** Whether items start expanded; defaults to true. */
  defaultExpanded?: boolean;
  isPinned: (nodeId: string) => boolean;
  onSelect: (nodeId: string) => void;
  /** Pins or unpins the given node ids, depending on `action`. */
  onAction: (nodeIds: string[]) => void;
}

export function nodeTitle(intl: IntlShape, node: SpecNode): string {
  return node.kind === "nospec"
    ? intl.formatMessage(messages.nospecPrefix, { title: node.title })
    : node.title;
}

function SnippetText({ snippet }: { snippet: Snippet }) {
  const parts: React.ReactNode[] = [];
  let cursor = 0;
  for (const [start, end] of snippet.highlights) {
    parts.push(snippet.text.slice(cursor, start));
    parts.push(<mark key={start}>{snippet.text.slice(start, end)}</mark>);
    cursor = end;
  }
  parts.push(snippet.text.slice(cursor));
  return <span className="sv-snippet">{parts}</span>;
}

function NodeName({
  intl,
  treeNode,
}: {
  intl: IntlShape;
  treeNode: SpecTreeNode;
}) {
  const { node, snippet, isEntry } = treeNode;
  return (
    <span className={isEntry ? "sv-node" : "sv-node sv-node--context"}>
      <span className="sv-node__title">
        <NodeLabels node={node} />
        <span>{nodeTitle(intl, node)}</span>
      </span>
      {snippet ? <SnippetText snippet={snippet} /> : null}
    </span>
  );
}

function entryIds(nodes: SpecTreeNode[]): string[] {
  return nodes.flatMap((child) => [
    ...(child.isEntry ? [child.node.id] : []),
    ...entryIds(child.children),
  ]);
}

export function SpecTreeView({
  idPrefix,
  label,
  files,
  selectedNodeId,
  action,
  defaultExpanded = true,
  isPinned,
  onSelect,
  onAction,
}: SpecTreeViewProps) {
  const intl = useIntl();
  const domId = (nodeId: string) => `${idPrefix}:${nodeId}`;
  const nodeIdByDomId = new Map<string, string>();

  const actionButton = (title: string, nodeIds: string[], pinned: boolean) => {
    const isPinAction = action === "pin";
    const message = isPinAction && !pinned ? messages.pin : messages.unpin;
    return (
      <Button
        aria-label={intl.formatMessage(message, { title })}
        {...(isPinAction ? { "aria-pressed": pinned, isClicked: pinned } : {})}
        icon={isPinAction ? <ThumbtackIcon /> : <TimesIcon />}
        onClick={() => {
          onAction(nodeIds);
        }}
        variant="plain"
      />
    );
  };

  const toItem = (treeNode: SpecTreeNode): TreeViewDataItem => {
    const id = domId(treeNode.node.id);
    nodeIdByDomId.set(id, treeNode.node.id);
    const title = nodeTitle(intl, treeNode.node);
    const ids = [treeNode.node.id, ...entryIds(treeNode.children)];
    return {
      id,
      name: <NodeName intl={intl} treeNode={treeNode} />,
      defaultExpanded,
      action: actionButton(title, ids, ids.every(isPinned)),
      ...(treeNode.children.length > 0
        ? { children: treeNode.children.map(toItem) }
        : {}),
    };
  };

  const data = files.map((treeFile): TreeViewDataItem => {
    const id = domId(`file:${treeFile.file.path}`);
    nodeIdByDomId.set(id, fileEntryId(treeFile.file));
    const ids = entryIds(treeFile.children);
    return {
      id,
      name: (
        <span className="sv-node">
          <span className="sv-node__title sv-node__title--file">
            {treeFile.file.title}
          </span>
          <span className="sv-node__path" title={treeFile.file.path}>
            {treeFile.file.path}
          </span>
        </span>
      ),
      defaultExpanded,
      action: actionButton(
        treeFile.file.title,
        ids,
        ids.length > 0 && ids.every(isPinned),
      ),
      children: treeFile.children.map(toItem),
    };
  });

  return (
    <TreeView
      aria-label={label}
      activeItems={
        selectedNodeId ? [{ id: domId(selectedNodeId), name: "" }] : []
      }
      data={data}
      hasGuides
      hasSelectableNodes
      onSelect={(_event, item) => {
        const nodeId = item.id ? nodeIdByDomId.get(item.id) : undefined;
        if (nodeId) onSelect(nodeId);
      }}
      className="sv-tree"
    />
  );
}
