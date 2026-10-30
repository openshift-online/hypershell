// Application-owned ports. Adapters in src/adapters implement them; the UI
// receives them through the composition root in src/main.tsx.

import type { FileReferences, SourceLanguage } from "../domain/references";
import type { MarkdownBlock, SpecFileSource } from "../domain/spec-document";
import type { MarkerLine } from "../domain/traceability";

/**
 * A branch of the repository. "" asks the server for its default: the
 * working tree in local development, the default branch in the dashboard.
 */
export type RefName = string;

export interface RefList {
  default: RefName;
  refs: RefName[];
}

export interface SpecSourcePort {
  /** Reads every markdown file under the specs root of `ref`. */
  loadSpecFiles(ref: RefName): Promise<SpecFileSource[]>;
  /** Calls `onChange` when spec files change; returns an unsubscribe function. */
  subscribe(onChange: () => void): () => void;
}

export interface MarkdownPort {
  /** Splits markdown into its top-level blocks. */
  lex(source: string): MarkdownBlock[];
  /**
   * Renders markdown to HTML. The n-th top-level heading (matching `lex`)
   * receives the element id `headingId(n)`.
   */
  render(source: string, headingId: (index: number) => string): string;
}

export interface PinStorePort {
  load(): string[];
  save(nodeIds: string[]): void;
}

export interface RepositoryPort {
  /** The branches the viewer can show, and the default one. */
  listRefs(): Promise<RefList>;
  /** Files in `ref` that mention a requirement or scenario ID. */
  findReferences(ref: RefName, id: string): Promise<FileReferences[]>;
  /** Text of a file in `ref`. */
  readFile(ref: RefName, path: string): Promise<string>;
  /** Every line of `ref` that starts a `Verifies:` test marker comment. */
  loadMarkers(ref: RefName): Promise<MarkerLine[]>;
}

export interface SyntaxHighlighterPort {
  /**
   * Renders source code as HTML with one `<span class="line">` per line,
   * colored for the given language.
   */
  highlight(code: string, language: SourceLanguage): Promise<string>;
}

export interface SpecViewerPorts {
  specs: SpecSourcePort;
  markdown: MarkdownPort;
  pins: PinStorePort;
  repository: RepositoryPort;
  highlighter: SyntaxHighlighterPort;
}
