import { readdir, readFile } from "node:fs/promises";
import path from "node:path";

export interface SpecFilePayload {
  path: string;
  content: string;
}

// Generated or vendored trees that hold no hand-written specs.
const SKIPPED_DIRECTORIES = new Set(["node_modules", "storybook-static"]);

/** Reads every markdown file under `root`, sorted by relative path. */
export async function readSpecFiles(root: string): Promise<SpecFilePayload[]> {
  const entries = await readdir(root, { recursive: true, withFileTypes: true });
  const files = entries.filter((entry) => {
    if (!entry.isFile() || !entry.name.endsWith(".md")) return false;
    const relative = path.relative(root, entry.parentPath);
    return !relative
      .split(path.sep)
      .some((segment) => SKIPPED_DIRECTORIES.has(segment));
  });

  const payload = await Promise.all(
    files.map(async (entry) => {
      const absolute = path.join(entry.parentPath, entry.name);
      return {
        path: path.relative(root, absolute).split(path.sep).join("/"),
        content: await readFile(absolute, "utf8"),
      };
    }),
  );
  return payload.sort((a, b) => a.path.localeCompare(b.path));
}

export function isSpecFile(root: string, file: string): boolean {
  const relative = path.relative(root, file);
  return (
    file.endsWith(".md") &&
    !relative.startsWith("..") &&
    !path.isAbsolute(relative)
  );
}

/**
 * Markdown files under `prefix` (such as "specs/") in a file list, with
 * paths relative to it: the same shape `readSpecFiles` returns for a folder.
 */
export async function specFilesFromTree(
  tree: {
    listFiles(): Promise<string[]>;
    load(file: string): Promise<string | null>;
  },
  prefix: string,
): Promise<SpecFilePayload[]> {
  const payload: SpecFilePayload[] = [];
  for (const file of await tree.listFiles()) {
    if (!file.startsWith(prefix) || !file.endsWith(".md")) continue;
    const relative = file.slice(prefix.length);
    if (
      relative
        .split("/")
        .slice(0, -1)
        .some((segment) => SKIPPED_DIRECTORIES.has(segment))
    ) {
      continue;
    }
    const content = await tree.load(file);
    if (content !== null) payload.push({ path: relative, content });
  }
  return payload.sort((a, b) => a.path.localeCompare(b.path));
}
