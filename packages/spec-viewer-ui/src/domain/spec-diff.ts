// Branch comparison. Two spec catalogs, the branch being compared against
// and the current branch, become one tree of what differs: files, then the
// requirements, scenarios, and plain-text sections inside them.
//
// Items are matched by ID, so a requirement whose title changed is one
// changed item, not a removed and a new one. An item without an ID is
// matched by its file and heading. Folders and files show only when they
// hold a difference, and an unchanged requirement shows only as the parent
// of a changed scenario.

import type { SpecCatalog, SpecFile, SpecNode } from "./spec-document";
import { textsDiffer } from "./text-diff";

export type DiffStatus = "added" | "removed" | "changed" | "unchanged";

/**
 * The catalog an entry is read from. A removed item exists only in the
 * branch being compared against; everything else is read from the current
 * branch.
 */
export type DiffSide = "current" | "compare";

export interface DiffEntry {
  /** Unique among the entries of one diff. */
  key: string;
  kind: "folder" | "file" | "item";
  /** Folders, and files and items kept as context, are "unchanged". */
  status: DiffStatus;
  /** Folder path, or file path; the file path of an item. */
  path: string;
  /** Display title of a folder or file; items are titled from `node`. */
  title: string;
  side: DiffSide;
  /** The file, for a file entry and for an item. */
  file?: SpecFile;
  /** The requirement, scenario, or plain-text heading of an item. */
  node?: SpecNode;
  /** Markdown of a changed item in the compared branch, and in this one. */
  before?: string;
  after?: string;
  children: DiffEntry[];
}

export interface DiffCounts {
  added: number;
  removed: number;
  changed: number;
}

export interface SpecDiff {
  /** Top-level folders and files. */
  entries: DiffEntry[];
  counts: DiffCounts;
  byKey: Map<string, DiffEntry>;
}

/** One comparable piece of a file: a node and the text it owns. */
interface Unit {
  node: SpecNode;
  file: SpecFile;
  /** Markdown of the heading and the text up to the next heading. */
  text: string;
  /** Equal for the same item in both branches. */
  matchKey: string;
}

function unitText(
  file: SpecFile,
  ownerId: string,
  nodesById: Map<string, SpecNode>,
): string {
  let text = "";
  for (const section of file.sections) {
    if (section.ownerId !== ownerId) continue;
    const node = nodesById.get(section.nodeId);
    // The preamble has no heading line of its own.
    if (node && node.headingIndex !== null) {
      // The lexer keeps some blank lines after a heading and drops others.
      text += `${"#".repeat(node.depth)} ${section.heading}\n\n`;
    }
    text += section.body;
  }
  return text;
}

/**
 * The units of a catalog by file. A plain heading inside a requirement or
 * scenario belongs to it, so only the others are units of their own.
 */
function unitsOf(catalog: SpecCatalog): Map<string, Unit[]> {
  const idCounts = new Map<string, number>();
  for (const node of catalog.nodesById.values()) {
    if (node.specId) {
      idCounts.set(node.specId, (idCounts.get(node.specId) ?? 0) + 1);
    }
  }
  const byFile = new Map<string, Unit[]>();
  for (const file of catalog.files) {
    const nodesById = new Map(file.nodes.map((node) => [node.id, node]));
    const ownerOf = new Map(
      file.sections.map((section) => [section.nodeId, section.ownerId]),
    );
    const units: Unit[] = [];
    for (const node of file.nodes) {
      const owner = ownerOf.get(node.id);
      if (owner !== undefined && owner !== node.id) continue;
      units.push({
        node,
        file,
        text: unitText(file, node.id, nodesById),
        matchKey:
          node.specId && idCounts.get(node.specId) === 1
            ? `id:${node.specId}`
            : `node:${node.id}`,
      });
    }
    byFile.set(file.path, units);
  }
  return byFile;
}

function byMatchKey(units: Map<string, Unit[]>): Map<string, Unit> {
  const all = new Map<string, Unit>();
  for (const list of units.values()) {
    for (const unit of list) all.set(unit.matchKey, unit);
  }
  return all;
}

/** An item before it is nested under its parent. */
interface FlatItem {
  entry: DiffEntry;
  matchKey: string;
  parentMatchKey: string | null;
}

const itemKey = (side: DiffSide, node: SpecNode): string =>
  `item:${side}:${node.id}`;

function item(
  side: DiffSide,
  unit: Unit,
  status: DiffStatus,
  other?: Unit,
): DiffEntry {
  return {
    key: itemKey(side, unit.node),
    kind: "item",
    status,
    path: unit.file.path,
    title: unit.node.title,
    side,
    file: unit.file,
    node: unit.node,
    ...(status === "changed" && other
      ? { before: other.text, after: unit.text }
      : {}),
    children: [],
  };
}

/** The items of one file that differ, nested, with unchanged parents kept. */
function fileItems(
  currentUnits: Unit[],
  compareUnits: Unit[],
  currentByKey: Map<string, Unit>,
  compareByKey: Map<string, Unit>,
): DiffEntry[] {
  const parentKeyOf = (unit: Unit, units: Unit[]): string | null => {
    const parentId = unit.node.parentId;
    return (
      units.find((candidate) => candidate.node.id === parentId)?.matchKey ??
      null
    );
  };

  const flat: FlatItem[] = currentUnits.map((unit) => {
    const before = compareByKey.get(unit.matchKey);
    let status: DiffStatus = "unchanged";
    if (!before) status = "added";
    else if (textsDiffer(before.text, unit.text)) status = "changed";
    return {
      entry: item("current", unit, status, before),
      matchKey: unit.matchKey,
      parentMatchKey: parentKeyOf(unit, currentUnits),
    };
  });

  // A removed item goes after the item that preceded it in its old file.
  compareUnits.forEach((unit, index) => {
    if (currentByKey.has(unit.matchKey)) return;
    let at = 0;
    for (let before = index - 1; before >= 0; before -= 1) {
      const position = flat.findIndex(
        (candidate) => candidate.matchKey === compareUnits[before]?.matchKey,
      );
      if (position !== -1) {
        at = position + 1;
        break;
      }
    }
    flat.splice(at, 0, {
      entry: item("compare", unit, "removed"),
      matchKey: unit.matchKey,
      parentMatchKey: parentKeyOf(unit, compareUnits),
    });
  });

  const byKey = new Map(flat.map((entry) => [entry.matchKey, entry]));
  const roots: DiffEntry[] = [];
  for (const flatItem of flat) {
    const parent =
      flatItem.parentMatchKey === null
        ? undefined
        : byKey.get(flatItem.parentMatchKey);
    if (parent && parent !== flatItem)
      parent.entry.children.push(flatItem.entry);
    else roots.push(flatItem.entry);
  }
  return roots.flatMap(function prune(entry): DiffEntry[] {
    entry.children = entry.children.flatMap(prune);
    return entry.status === "unchanged" && entry.children.length === 0
      ? []
      : [entry];
  });
}

interface FolderBuilder {
  path: string;
  folders: Map<string, FolderBuilder>;
  files: DiffEntry[];
}

function folderEntry(folder: FolderBuilder): DiffEntry {
  const byName = (a: DiffEntry, b: DiffEntry) => a.path.localeCompare(b.path);
  return {
    key: `folder:${folder.path}`,
    kind: "folder",
    status: "unchanged",
    path: folder.path,
    title: folder.path,
    side: "current",
    children: [
      ...[...folder.folders.values()].map(folderEntry).sort(byName),
      ...[...folder.files].sort(byName),
    ],
  };
}

function countItems(entries: DiffEntry[], counts: DiffCounts): void {
  for (const entry of entries) {
    if (entry.kind === "item" && entry.status !== "unchanged") {
      counts[entry.status] += 1;
    }
    countItems(entry.children, counts);
  }
}

function indexEntries(
  entries: DiffEntry[],
  into: Map<string, DiffEntry>,
): void {
  for (const entry of entries) {
    into.set(entry.key, entry);
    indexEntries(entry.children, into);
  }
}

/** What differs between `compare` (the older side) and `current`. */
export function diffCatalogs(
  compare: SpecCatalog,
  current: SpecCatalog,
): SpecDiff {
  const compareUnits = unitsOf(compare);
  const currentUnits = unitsOf(current);
  const compareByKey = byMatchKey(compareUnits);
  const currentByKey = byMatchKey(currentUnits);

  const root: FolderBuilder = { path: "", folders: new Map(), files: [] };
  const paths = new Set([
    ...compare.filesByPath.keys(),
    ...current.filesByPath.keys(),
  ]);
  for (const path of [...paths].sort()) {
    const currentFile = current.filesByPath.get(path);
    const compareFile = compare.filesByPath.get(path);
    const file = currentFile ?? compareFile;
    if (!file) continue;
    const children = fileItems(
      currentUnits.get(path) ?? [],
      compareUnits.get(path) ?? [],
      currentByKey,
      compareByKey,
    );
    const status: DiffStatus = !compareFile
      ? "added"
      : !currentFile
        ? "removed"
        : "unchanged";
    if (status === "unchanged" && children.length === 0) continue;

    let folder = root;
    const segments = path.split("/").slice(0, -1);
    segments.forEach((name, index) => {
      let child = folder.folders.get(name);
      if (!child) {
        child = {
          path: segments.slice(0, index + 1).join("/"),
          folders: new Map(),
          files: [],
        };
        folder.folders.set(name, child);
      }
      folder = child;
    });
    const entry: DiffEntry = {
      key: `file:${path}`,
      kind: "file",
      status,
      path,
      title: file.title,
      side: currentFile ? "current" : "compare",
      file,
      children,
    };
    folder.files.push(entry);
  }

  const entries = folderEntry(root).children;
  const counts: DiffCounts = { added: 0, removed: 0, changed: 0 };
  countItems(entries, counts);
  const byKey = new Map<string, DiffEntry>();
  indexEntries(entries, byKey);
  return { entries, counts, byKey };
}

/** True when the branches differ in no spec file. */
export function isEmptyDiff(diff: SpecDiff): boolean {
  return diff.entries.length === 0;
}
