import { execFile, spawn } from "node:child_process";
import { readFile, stat } from "node:fs/promises";
import path from "node:path";
import { promisify } from "node:util";

const run = promisify(execFile);

export interface ReferenceMatch {
  /** 1-based line number. */
  line: number;
  text: string;
}

export interface FileReferences {
  /** Path relative to the repository root, with forward slashes. */
  path: string;
  /** True for test files, which give a requirement its traceability. */
  isTest: boolean;
  matches: ReferenceMatch[];
}

/** A line that starts a `Verifies:` comment (see src/domain/traceability). */
export interface MarkerLine {
  path: string;
  line: number;
  text: string;
  /** Following lines, up to and including the first code line. */
  following: string[];
}

/** The pseudo-branch for the files on disk, with uncommitted edits. */
export const WORKING_TREE = "@";

export interface RefList {
  default: string;
  refs: string[];
}

/** The text files of one version of the repository. */
export interface FileTree {
  /** Every file of this version, sorted. */
  listFiles(): Promise<string[]>;
  /** Text of a file; null when it is missing, binary, or over 1 MB. */
  load(file: string): Promise<string | null>;
}

export interface Repository {
  /** The working tree and the local branches. */
  listRefs(): Promise<RefList>;
  /** Files of `ref`; throws `UnknownRefError` for anything not listed. */
  tree(ref: string): Promise<FileTree>;
}

export class UnknownRefError extends Error {
  constructor(ref: string) {
    super(`Unknown branch: ${ref}`);
    this.name = "UnknownRefError";
  }
}

/** Larger files are generated or vendored; skip them. */
const MAX_FILE_BYTES = 1_000_000;
const MAX_MATCHES_PER_FILE = 50;
const MAX_LINE_LENGTH = 300;
export const ID_PATTERN = /^[A-Z][A-Z0-9]*(?:[-.][A-Z0-9]+)+$/u;

const TEST_FILE = /(?:_test\.go|\.(?:test|spec)\.[cm]?[jt]sx?|_test\.sh)$/u;
const TEST_FOLDER = /(?:^|\/)(?:tests?|e2e|e2e-live|__tests__)\//u;

// Same scope as scripts/check_spec_ids.py: lockfiles hold digests that can
// look like IDs, and markdown holds no markers.
const MARKER_SKIPPED_NAMES = new Set([
  "pnpm-lock.yaml",
  "package-lock.json",
  "yarn.lock",
  "go.sum",
]);
const MARKER_SKIPPED_PREFIXES = ["components/agents/locks/"];
const MARKER_START = /^\s*(?:\/\/|#)\s*Verifies:/u;
const MAX_FOLLOWING_LINES = 40;
const COMMENT_STARTS = ["//", "#", "/*", "*", "*/"];

export function isMarkerCandidate(file: string): boolean {
  return (
    !file.endsWith(".md") &&
    !MARKER_SKIPPED_NAMES.has(file.slice(file.lastIndexOf("/") + 1)) &&
    !MARKER_SKIPPED_PREFIXES.some((prefix) => file.startsWith(prefix))
  );
}

/** Marker lines of one file, each with the lines up to its test. */
export function markerLines(file: string, text: string): MarkerLine[] {
  if (!text.includes("Verifies:")) return [];
  const lines = text.split("\n");
  const found: MarkerLine[] = [];
  lines.forEach((line, index) => {
    if (!MARKER_START.test(line)) return;
    const following: string[] = [];
    for (const next of lines.slice(index + 1)) {
      if (following.length >= MAX_FOLLOWING_LINES) break;
      following.push(next.slice(0, MAX_LINE_LENGTH));
      const stripped = next.trim();
      const isComment =
        stripped === "" ||
        COMMENT_STARTS.some((start) => stripped.startsWith(start));
      if (!isComment) break;
    }
    found.push({ path: file, line: index + 1, text: line, following });
  });
  return found;
}

export function isTestFile(file: string): boolean {
  return (
    !file.endsWith(".md") && (TEST_FILE.test(file) || TEST_FOLDER.test(file))
  );
}

const escapeRegExp = (text: string): string =>
  text.replace(/[.*+?^${}()|[\]\\]/gu, "\\$&");

/**
 * Matches `id` as a whole identifier: `ABC-DEF-01` matches "ABC-DEF-01 --" and
 * "(ABC-DEF-01)", but not "ABC-DEF-010", "XABC-DEF-01", or the scenario ID "ABC-DEF-01.01".
 */
export function idPattern(id: string): RegExp {
  return new RegExp(
    `(?<![A-Za-z0-9_.-])${escapeRegExp(id)}(?![A-Za-z0-9_]|[-.][A-Za-z0-9])`,
    "u",
  );
}

export async function repositoryRoot(directory: string): Promise<string> {
  const { stdout } = await run("git", ["rev-parse", "--show-toplevel"], {
    cwd: directory,
  });
  return stdout.trim();
}

interface CachedText {
  mtimeMs: number;
  text: string | null;
}

/** Folders that hold no hand-written repository content. */
const SKIPPED_TREE_DIRECTORIES = new Set([".git", "node_modules"]);
/** Branch snapshots kept in memory, by commit. */
const MAX_CACHED_COMMITS = 3;

const isBinary = (content: Buffer): boolean =>
  content.subarray(0, 8000).includes(0);

/** Files on disk that git lists: tracked, or untracked but not ignored. */
function workingTree(root: string): FileTree {
  const cache = new Map<string, CachedText>();
  return {
    async listFiles() {
      const { stdout } = await run(
        "git",
        ["ls-files", "--cached", "--others", "--exclude-standard", "-z"],
        { cwd: root, maxBuffer: 64 * 1024 * 1024 },
      );
      return [
        ...new Set(stdout.split("\0").filter((file) => file !== "")),
      ].sort();
    },
    /** Reads `file` through an mtime-checked cache. */
    async load(file) {
      const absolute = path.join(root, file);
      let info;
      try {
        info = await stat(absolute);
      } catch {
        // Listed but deleted since: nothing to read.
        return null;
      }
      if (!info.isFile()) return null;
      const cached = cache.get(file);
      if (cached?.mtimeMs === info.mtimeMs) return cached.text;
      let text: string | null = null;
      if (info.size <= MAX_FILE_BYTES) {
        const content = await readFile(absolute);
        text = isBinary(content) ? null : content.toString("utf8");
      }
      cache.set(file, { mtimeMs: info.mtimeMs, text });
      return text;
    },
  };
}

/** Runs git with `input` on stdin and returns stdout as bytes. */
function gitBytes(
  root: string,
  args: string[],
  input: string,
): Promise<Buffer> {
  return new Promise((resolve, reject) => {
    const child = spawn("git", args, { cwd: root });
    const chunks: Buffer[] = [];
    let stderr = "";
    child.stdout.on("data", (chunk: Buffer) => chunks.push(chunk));
    child.stderr.on("data", (chunk: Buffer) => {
      stderr += chunk.toString();
    });
    child.on("error", reject);
    child.on("close", (code) => {
      if (code === 0) resolve(Buffer.concat(chunks));
      else reject(new Error(`git ${args[0] ?? ""} failed: ${stderr.trim()}`));
    });
    child.stdin.end(input);
  });
}

/** Reads the text files of one commit into memory. */
async function commitTree(root: string, commit: string): Promise<FileTree> {
  const { stdout } = await run("git", ["ls-tree", "-r", "-z", "-l", commit], {
    cwd: root,
    maxBuffer: 64 * 1024 * 1024,
  });
  const blobs: { object: string; file: string }[] = [];
  for (const record of stdout.split("\0")) {
    // "<mode> <type> <object> <size>\t<path>"; the size is space-padded.
    const tab = record.indexOf("\t");
    if (tab === -1) continue;
    const [mode, type, object, size] = record.slice(0, tab).split(/\s+/u);
    const file = record.slice(tab + 1);
    // Regular files only: symlinks and submodules are skipped.
    if (type !== "blob" || (mode !== "100644" && mode !== "100755")) continue;
    if (!object || Number(size) > MAX_FILE_BYTES) continue;
    if (file.split("/").some((part) => SKIPPED_TREE_DIRECTORIES.has(part))) {
      continue;
    }
    blobs.push({ object, file });
  }
  const output = await gitBytes(
    root,
    ["cat-file", "--batch"],
    blobs.map((blob) => blob.object).join("\n") + "\n",
  );
  const texts = new Map<string, string>();
  let offset = 0;
  for (const blob of blobs) {
    // Each answer is "<object> blob <size>\n<content>\n".
    const headerEnd = output.indexOf(10, offset);
    const size = Number(
      output.subarray(offset, headerEnd).toString().split(" ")[2],
    );
    const content = output.subarray(headerEnd + 1, headerEnd + 1 + size);
    offset = headerEnd + 1 + size + 1;
    if (!isBinary(content)) texts.set(blob.file, content.toString("utf8"));
  }
  const files = [...texts.keys()].sort();
  return {
    listFiles: () => Promise.resolve(files),
    load: (file) => Promise.resolve(texts.get(file) ?? null),
  };
}

export function createRepository(root: string): Repository {
  const working = workingTree(root);
  // Branch snapshots by commit, so a branch that moves is read again.
  const commits = new Map<string, Promise<FileTree>>();

  const listRefs = async (): Promise<RefList> => {
    const { stdout } = await run(
      "git",
      ["for-each-ref", "--format=%(refname:short)", "refs/heads"],
      { cwd: root },
    );
    const branches = stdout.split("\n").filter((name) => name !== "");
    return { default: WORKING_TREE, refs: [WORKING_TREE, ...branches.sort()] };
  };

  return {
    listRefs,
    async tree(ref) {
      if (ref === WORKING_TREE) return working;
      // Only listed branches reach git, so a ref cannot be read as an option.
      if (!(await listRefs()).refs.includes(ref))
        throw new UnknownRefError(ref);
      const { stdout } = await run(
        "git",
        ["rev-parse", "--verify", `refs/heads/${ref}^{commit}`],
        { cwd: root },
      );
      const commit = stdout.trim();
      let tree = commits.get(commit);
      if (!tree) {
        tree = commitTree(root, commit);
        commits.set(commit, tree);
        tree.catch(() => commits.delete(commit));
        while (commits.size > MAX_CACHED_COMMITS) {
          const oldest = commits.keys().next().value;
          if (oldest === undefined) break;
          commits.delete(oldest);
        }
      }
      return tree;
    },
  };
}

/** Text of a file the tree lists; null otherwise. */
export async function readText(
  tree: FileTree,
  file: string,
): Promise<string | null> {
  return (await tree.listFiles()).includes(file) ? tree.load(file) : null;
}

/** Every `Verifies:` marker line outside markdown and lockfiles. */
export async function findMarkers(tree: FileTree): Promise<MarkerLine[]> {
  const found: MarkerLine[] = [];
  for (const file of await tree.listFiles()) {
    if (!isMarkerCandidate(file)) continue;
    const text = await tree.load(file);
    if (text) found.push(...markerLines(file, text));
  }
  return found;
}

/** Files whose text mentions `id` as a whole identifier, tests first. */
export async function findReferences(
  tree: FileTree,
  id: string,
): Promise<FileReferences[]> {
  const pattern = idPattern(id);
  const results: FileReferences[] = [];
  for (const file of await tree.listFiles()) {
    const text = await tree.load(file);
    if (!text?.includes(id)) continue;
    const matches: ReferenceMatch[] = [];
    const lines = text.split("\n");
    for (let index = 0; index < lines.length; index += 1) {
      const line = lines[index] ?? "";
      if (!pattern.test(line)) continue;
      matches.push({
        line: index + 1,
        text: line.trim().slice(0, MAX_LINE_LENGTH),
      });
      if (matches.length >= MAX_MATCHES_PER_FILE) break;
    }
    if (matches.length > 0) {
      results.push({ path: file, isTest: isTestFile(file), matches });
    }
  }
  // Tests first: they are what traceability is about.
  return results.sort(
    (a, b) =>
      Number(b.isTest) - Number(a.isTest) || a.path.localeCompare(b.path),
  );
}
