// Spec document model. A spec file is split into headings; each heading is a
// navigable node classified as a spec (requirement), a scenario, or a plain
// markdown heading ("nospec"). Text between headings becomes a searchable
// section owned by the nearest spec or scenario, or by its own heading when
// no spec or scenario encloses it.

/** One top-level markdown block, as produced by the markdown lexer port. */
export type MarkdownBlock =
  | { kind: "heading"; depth: number; text: string; raw: string }
  | { kind: "other"; raw: string };

export interface SpecFileSource {
  /** Path relative to the specs root, using forward slashes. */
  path: string;
  content: string;
}

export type SpecNodeKind = "spec" | "scenario" | "nospec";

export interface SpecNode {
  /** Stable id: `<path>#<heading key>`, or `<path>#` for the file preamble. */
  id: string;
  path: string;
  kind: SpecNodeKind;
  /** Display title with markup and `Requirement:`/`Scenario:` prefixes removed. */
  title: string;
  /** Heading text as written, without markup; the file title for the preamble. */
  heading: string;
  /**
   * Identifier from the heading, when it has one: `QRX-01` for a requirement,
   * `QRX-01.01` for a scenario.
   */
  specId?: string;
  /** Index of the heading among the file's top-level headings; null for the preamble. */
  headingIndex: number | null;
  depth: number;
  /** 1-based source line of the heading. */
  line: number;
  /** Spec that encloses a scenario, or the heading that encloses an orphan scenario. */
  parentId: string | null;
}

export interface SpecSection {
  /** Node that search hits in this section are reported against. */
  ownerId: string;
  /** Node whose heading starts this section. */
  nodeId: string;
  heading: string;
  body: string;
}

export interface SpecFile {
  path: string;
  title: string;
  /** Markdown source, reassembled from the lexed blocks. */
  content: string;
  nodes: SpecNode[];
  sections: SpecSection[];
}

const SCENARIO_PATTERN = /^Scenario(?:\s+Outline)?\s*[:-]\s*(.*)$/iu;
// "Requirement: Title" or "Requirement QRX-01: Title"; a heading such as
// "Requirement and Scenario IDs" is prose, not a requirement.
const REQUIREMENT_PATTERN =
  /^Requirement(?:\s*:|\s+(?=[A-Z][A-Z0-9]*-[A-Z0-9]))\s*(.*)$/u;
// Requirement identifiers end in a segment that carries a digit: QRX-00,
// ZZ-A11Y-01, ZZW-UI-03A.
const SPEC_ID_PATTERN =
  /^([A-Z][A-Z0-9]*(?:-[A-Z0-9]+)*-[A-Z]*\d+[A-Z0-9]*)\b\s*(?:--|:|-|–)?\s*(.*)$/u;
// A scenario ID leads the scenario title and must be followed by `--` or `:`
// (`Scenario: QRX-01.01 -- Title`), so titles that merely start with an
// ID-like word, such as "HTTP-2 upgrade", are not mistaken for IDs.
const SCENARIO_ID_PATTERN =
  /^([A-Z][A-Z0-9]*(?:[-.][A-Z0-9]+)*[-.][A-Z]*\d+[A-Z0-9]*)\s*(?:--|:|\u2013)\s*(.+)$/u;

/** Removes inline markdown emphasis and code markers from heading text. */
export function plainHeading(text: string): string {
  return text
    .replace(/\[([^\]]*)\]\([^)]*\)/gu, "$1")
    .replace(/[`*_]/gu, "")
    .trim();
}

function nonEmpty(value: string | undefined, fallback: string): string {
  const trimmed = value?.trim() ?? "";
  return trimmed === "" ? fallback : trimmed;
}

interface Classified {
  kind: SpecNodeKind;
  title: string;
  specId?: string;
}

export function classifyHeading(text: string, depth: number): Classified {
  const plain = plainHeading(text);
  const scenario = SCENARIO_PATTERN.exec(plain);
  if (scenario) {
    const title = nonEmpty(scenario[1], plain);
    const id = SCENARIO_ID_PATTERN.exec(title);
    return id?.[1] && id[2]
      ? { kind: "scenario", specId: id[1], title: id[2].trim() }
      : { kind: "scenario", title };
  }
  const requirement = REQUIREMENT_PATTERN.exec(plain);
  if (requirement) {
    return { kind: "spec", ...splitSpecId(nonEmpty(requirement[1], plain)) };
  }
  if (depth > 1) {
    const id = SPEC_ID_PATTERN.exec(plain);
    if (id?.[1]) {
      return { kind: "spec", specId: id[1], title: nonEmpty(id[2], id[1]) };
    }
  }
  return { kind: "nospec", title: plain };
}

function splitSpecId(text: string): { title: string; specId?: string } {
  const id = SPEC_ID_PATTERN.exec(text);
  if (id?.[1]) {
    return { specId: id[1], title: nonEmpty(id[2], id[1]) };
  }
  return { title: text };
}

function fileName(path: string): string {
  return path.split("/").pop() ?? path;
}

function countLines(text: string): number {
  let lines = 0;
  for (const char of text) {
    if (char === "\n") lines += 1;
  }
  return lines;
}

interface OpenHeading {
  depth: number;
  node: SpecNode;
}

/** Builds the spec model of one file from its top-level markdown blocks. */
export function parseSpecFile(path: string, blocks: MarkdownBlock[]): SpecFile {
  const firstH1 = blocks.find(
    (block) => block.kind === "heading" && block.depth === 1,
  );
  const title =
    firstH1?.kind === "heading" ? plainHeading(firstH1.text) : fileName(path);

  const preamble: SpecNode = {
    id: `${path}#`,
    path,
    kind: "nospec",
    title,
    heading: title,
    headingIndex: null,
    depth: 0,
    line: 1,
    parentId: null,
  };
  const nodes: SpecNode[] = [];
  const sections: SpecSection[] = [];
  const keyCounts = new Map<string, number>();
  let current: SpecSection = {
    ownerId: preamble.id,
    nodeId: preamble.id,
    heading: title,
    body: "",
  };

  // Headings still open at the current position, outermost first.
  const open: OpenHeading[] = [];
  let line = 1;
  let headingIndex = 0;

  const flush = () => {
    if (current.nodeId === preamble.id && current.body.trim() === "") return;
    sections.push(current);
  };

  for (const block of blocks) {
    if (block.kind === "other") {
      current.body += block.raw;
      line += countLines(block.raw);
      continue;
    }

    flush();
    while (open.length > 0 && (open.at(-1)?.depth ?? 0) >= block.depth) {
      open.pop();
    }

    const classified = classifyHeading(block.text, block.depth);
    const baseKey = plainHeading(block.text).toLowerCase();
    const seen = keyCounts.get(baseKey) ?? 0;
    keyCounts.set(baseKey, seen + 1);
    const key = seen === 0 ? baseKey : `${baseKey}~${String(seen + 1)}`;

    const enclosingSpec = open.findLast((entry) => entry.node.kind === "spec");
    const enclosingScenario = open.findLast(
      (entry) => entry.node.kind === "scenario",
    );
    let parentId: string | null = null;
    if (classified.kind === "scenario") {
      parentId = (enclosingSpec ?? open.at(-1))?.node.id ?? null;
    }

    const node: SpecNode = {
      id: `${path}#${key}`,
      path,
      kind: classified.kind,
      title: classified.title,
      heading: plainHeading(block.text),
      ...(classified.specId ? { specId: classified.specId } : {}),
      headingIndex,
      depth: block.depth,
      line,
      parentId,
    };
    nodes.push(node);
    open.push({ depth: block.depth, node });

    let ownerId = node.id;
    if (node.kind === "nospec") {
      ownerId = (enclosingScenario ?? enclosingSpec)?.node.id ?? node.id;
    }
    current = { ownerId, nodeId: node.id, heading: block.text, body: "" };

    headingIndex += 1;
    line += countLines(block.raw);
  }
  flush();

  const preambleUsed = sections[0]?.nodeId === preamble.id;
  return {
    path,
    title,
    content: blocks.map((block) => block.raw).join(""),
    nodes: preambleUsed ? [preamble, ...nodes] : nodes,
    sections,
  };
}

export interface SpecCatalog {
  files: SpecFile[];
  nodesById: Map<string, SpecNode>;
  filesByPath: Map<string, SpecFile>;
}

export function buildCatalog(files: SpecFile[]): SpecCatalog {
  const sorted = [...files].sort((a, b) => a.path.localeCompare(b.path));
  const nodesById = new Map<string, SpecNode>();
  const filesByPath = new Map<string, SpecFile>();
  for (const file of sorted) {
    filesByPath.set(file.path, file);
    for (const node of file.nodes) nodesById.set(node.id, node);
  }
  return { files: sorted, nodesById, filesByPath };
}

/** Node id for a file as a whole: its first heading, or its preamble. */
export function fileEntryId(file: SpecFile): string {
  return file.nodes[0]?.id ?? `${file.path}#`;
}
