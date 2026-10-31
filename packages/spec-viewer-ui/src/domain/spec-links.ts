// Resolves links found in a spec file to nodes of the catalog, so the viewer
// can open linked specs in place. Handles relative markdown links with
// optional GitHub-style heading anchors, and bare spec file names written
// as inline code (`data-model.spec.md`), which is how most specs refer to
// each other.

import { fileEntryId, type SpecCatalog, type SpecFile } from "./spec-document";

export type LinkTarget =
  | { kind: "spec"; nodeId: string }
  | { kind: "external" }
  /** A relative link to something outside the specs folder. */
  | { kind: "unresolved"; path: string };

/** GitHub's heading anchor: lowercase, punctuation dropped, spaces to dashes. */
export function githubSlug(heading: string): string {
  return heading
    .toLowerCase()
    .trim()
    .replace(/[^\p{L}\p{N}\s_-]/gu, "")
    .replace(/\s/gu, "-");
}

const slugCache = new WeakMap<SpecFile, Map<string, string>>();

function slugsOf(file: SpecFile): Map<string, string> {
  let slugs = slugCache.get(file);
  if (slugs) return slugs;
  slugs = new Map();
  const seen = new Map<string, number>();
  for (const node of file.nodes) {
    if (node.headingIndex === null) continue;
    const base = githubSlug(node.heading);
    const count = seen.get(base) ?? 0;
    seen.set(base, count + 1);
    slugs.set(count === 0 ? base : `${base}-${String(count)}`, node.id);
  }
  slugCache.set(file, slugs);
  return slugs;
}

/** Joins `relative` onto the directory of `fromPath`; null above the root. */
export function resolvePath(fromPath: string, relative: string): string | null {
  const segments = relative.startsWith("/")
    ? []
    : fromPath.split("/").slice(0, -1);
  for (const segment of relative.split("/")) {
    if (segment === "" || segment === ".") continue;
    if (segment === "..") {
      if (segments.length === 0) return null;
      segments.pop();
    } else {
      segments.push(segment);
    }
  }
  return segments.join("/");
}

function decode(text: string): string {
  try {
    return decodeURIComponent(text);
  } catch {
    return text;
  }
}

function target(file: SpecFile, anchor: string): LinkTarget {
  const nodeId = anchor === "" ? undefined : slugsOf(file).get(anchor);
  return { kind: "spec", nodeId: nodeId ?? fileEntryId(file) };
}

/** Resolves an `href` written in the spec file at `fromPath`. */
export function resolveSpecLink(
  catalog: SpecCatalog,
  fromPath: string,
  href: string,
): LinkTarget {
  if (/^[a-z][a-z0-9+.-]*:/iu.test(href)) return { kind: "external" };
  const hashAt = href.indexOf("#");
  const pathPart = decode(hashAt === -1 ? href : href.slice(0, hashAt));
  const anchor = decode(hashAt === -1 ? "" : href.slice(hashAt + 1));

  const path = pathPart === "" ? fromPath : resolvePath(fromPath, pathPart);
  const file = path === null ? undefined : catalog.filesByPath.get(path);
  if (!file) return { kind: "unresolved", path: pathPart };
  return target(file, anchor.toLowerCase());
}

/**
 * Resolves a spec file named in inline code: relative to the current file,
 * then to the specs root, then by a file name that is unique in the specs.
 */
export function resolveSpecReference(
  catalog: SpecCatalog,
  fromPath: string,
  reference: string,
): string | null {
  const match = /^([\w./-]+\.md)(?:#([\w-]+))?$/u.exec(reference.trim());
  if (!match?.[1]) return null;
  const [, name, anchor = ""] = match;
  const relative = resolvePath(fromPath, name);
  let file =
    (relative === null ? undefined : catalog.filesByPath.get(relative)) ??
    catalog.filesByPath.get(name);
  if (!file && !name.includes("/")) {
    const sameName = catalog.files.filter(
      (candidate) => candidate.path.split("/").pop() === name,
    );
    if (sameName.length === 1) file = sameName[0];
  }
  if (!file) return null;
  const resolved = target(file, anchor.toLowerCase());
  return resolved.kind === "spec" ? resolved.nodeId : null;
}
