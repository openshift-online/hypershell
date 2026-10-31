import {
  buildCatalog,
  parseSpecFile,
  type SpecCatalog,
} from "../domain/spec-document";
import type { MarkdownPort, RefName, SpecSourcePort } from "./ports";

export async function loadSpecCatalog(
  specs: SpecSourcePort,
  markdown: MarkdownPort,
  ref: RefName,
): Promise<SpecCatalog> {
  const sources = await specs.loadSpecFiles(ref);
  return buildCatalog(
    sources.map((source) =>
      parseSpecFile(source.path, markdown.lex(source.content)),
    ),
  );
}
