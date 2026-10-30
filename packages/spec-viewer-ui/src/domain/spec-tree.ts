// Groups nodes (search hits or pinned entries) into the file > spec >
// scenario hierarchy shown in the results list and the pinned panel.

import type { SpecCatalog, SpecFile, SpecNode } from "./spec-document";
import type { Snippet } from "./search";

export interface SpecTreeNode {
  node: SpecNode;
  /** True when the node itself was requested; false for context parents. */
  isEntry: boolean;
  snippet?: Snippet;
  children: SpecTreeNode[];
}

export interface SpecTreeFile {
  file: SpecFile;
  children: SpecTreeNode[];
}

export interface SpecTreeEntry {
  nodeId: string;
  snippet?: Snippet;
}

/**
 * Builds the tree for `entries`. Files keep the order in which their first
 * entry appears (search rank); nodes within a file keep document order.
 * Entries whose ids no longer resolve are returned in `missing`.
 */
export function buildSpecTree(
  catalog: SpecCatalog,
  entries: SpecTreeEntry[],
): { files: SpecTreeFile[]; missing: string[] } {
  const missing: string[] = [];
  const fileOrder: string[] = [];
  const treeNodes = new Map<string, SpecTreeNode>();

  const ensure = (node: SpecNode): SpecTreeNode => {
    let treeNode = treeNodes.get(node.id);
    if (!treeNode) {
      treeNode = { node, isEntry: false, children: [] };
      treeNodes.set(node.id, treeNode);
      if (!fileOrder.includes(node.path)) fileOrder.push(node.path);
    }
    return treeNode;
  };

  for (const entry of entries) {
    const node = catalog.nodesById.get(entry.nodeId);
    if (!node) {
      missing.push(entry.nodeId);
      continue;
    }
    const treeNode = ensure(node);
    treeNode.isEntry = true;
    if (entry.snippet) treeNode.snippet = entry.snippet;
    if (node.parentId) {
      const parent = catalog.nodesById.get(node.parentId);
      if (parent) ensure(parent);
    }
  }

  const byFile = new Map<string, SpecTreeNode[]>();
  for (const treeNode of treeNodes.values()) {
    const parentId = treeNode.node.parentId;
    const parent = parentId ? treeNodes.get(parentId) : undefined;
    if (parent) {
      parent.children.push(treeNode);
    } else {
      const roots = byFile.get(treeNode.node.path) ?? [];
      roots.push(treeNode);
      byFile.set(treeNode.node.path, roots);
    }
  }

  const byLine = (a: SpecTreeNode, b: SpecTreeNode) =>
    a.node.line - b.node.line;
  const sortDeep = (nodes: SpecTreeNode[]) => {
    nodes.sort(byLine);
    for (const child of nodes) sortDeep(child.children);
  };

  const files: SpecTreeFile[] = [];
  for (const path of fileOrder) {
    const file = catalog.filesByPath.get(path);
    const children = byFile.get(path) ?? [];
    if (!file) continue;
    sortDeep(children);
    files.push({ file, children });
  }
  return { files, missing };
}
