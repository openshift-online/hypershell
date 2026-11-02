// Filters the conformance tree by a search and by starred items. The table
// shows a row when the row itself matches, or when something below it does;
// the rows above a match are expanded so every match is on screen.
//
// Row keys: "folder:<path>", "file:<path>", and node ids for requirements
// and scenarios.

import type {
  FileReport,
  FolderReport,
  RequirementReport,
} from "./conformance";
import type { SearchHit } from "./search";

export interface ConformanceFilter {
  /** Search hits by node id; undefined when nothing is searched. */
  hits?: ReadonlyMap<string, SearchHit>;
  /** Starred node ids; undefined when the star filter is off. */
  starred?: ReadonlySet<string>;
}

export interface FilteredConformance {
  /** Row keys to show. */
  visible: Set<string>;
  /** Row keys to expand so every match shows. */
  expanded: Set<string>;
  /**
   * Hits on text outside any requirement, by file path. Such text has no row
   * of its own, so the file row stands for it.
   */
  textHits: Map<string, SearchHit[]>;
}

export const folderKey = (path: string): string => `folder:${path}`;
export const fileKey = (path: string): string => `file:${path}`;

/** True when a search or the star filter narrows the tree. */
export function isFilterActive(filter: ConformanceFilter): boolean {
  return filter.hits !== undefined || filter.starred !== undefined;
}

/** Null when no filter is active: every row shows. */
export function filterConformance(
  root: FolderReport,
  filter: ConformanceFilter,
): FilteredConformance | null {
  if (!isFilterActive(filter)) return null;
  const { hits, starred } = filter;
  const searchOk = (id: string) => !hits || hits.has(id);
  const result: FilteredConformance = {
    visible: new Set(),
    expanded: new Set(),
    textHits: new Map(),
  };

  const show = (key: string, expand: boolean) => {
    result.visible.add(key);
    if (expand) result.expanded.add(key);
  };

  const requirement = (report: RequirementReport): boolean => {
    const id = report.node.id;
    const isStarred = starred?.has(id) ?? true;
    let anyScenario = false;
    for (const scenario of report.scenarios) {
      const sid = scenario.node.id;
      // A starred requirement brings all its scenarios with it.
      const starOk = !starred || isStarred || starred.has(sid);
      if (searchOk(sid) && starOk) {
        result.visible.add(sid);
        anyScenario = true;
      }
    }
    const direct = searchOk(id) && isStarred;
    if (!direct && !anyScenario) return false;
    show(id, anyScenario);
    return true;
  };

  const file = (report: FileReport): boolean => {
    // Text outside requirements: "[nospec]" sections and the preamble.
    const text = report.file.nodes.filter(
      (node) =>
        node.kind === "nospec" &&
        searchOk(node.id) &&
        (!starred || starred.has(node.id)),
    );
    const textHits = text.flatMap((node) => {
      const hit = hits?.get(node.id);
      return hit ? [hit] : [];
    });
    if (textHits.length > 0) result.textHits.set(report.file.path, textHits);
    let anyRequirement = false;
    for (const child of report.requirements) {
      if (requirement(child)) anyRequirement = true;
    }
    if (!anyRequirement && text.length === 0) return false;
    show(fileKey(report.file.path), anyRequirement);
    return true;
  };

  const folder = (report: FolderReport): boolean => {
    let any = false;
    for (const child of report.folders) {
      if (folder(child)) any = true;
    }
    for (const child of report.files) {
      if (file(child)) any = true;
    }
    if (any && report.path !== "") show(folderKey(report.path), true);
    return any;
  };

  folder(root);
  return result;
}
