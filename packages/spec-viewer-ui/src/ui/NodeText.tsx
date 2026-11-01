import type { IntlShape } from "react-intl";

import type { Snippet } from "../domain/search";
import type { SpecNode } from "../domain/spec-document";
import { messages } from "../messages";

export function nodeTitle(intl: IntlShape, node: SpecNode): string {
  return node.kind === "nospec"
    ? intl.formatMessage(messages.nospecPrefix, { title: node.title })
    : node.title;
}

export function SnippetText({ snippet }: { snippet: Snippet }) {
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
