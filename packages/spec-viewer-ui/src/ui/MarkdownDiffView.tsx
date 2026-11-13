import { Content } from "@patternfly/react-core";
import type { Root } from "hast";
import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";

import type { DiffSegment } from "../domain/text-diff";
import { markSegments } from "./hast-diff";

interface MarkdownDiffViewProps {
  source: string;
  segments: DiffSegment[];
}

/** Renders one side of a Markdown diff with changed words marked. */
export function MarkdownDiffView({ source, segments }: MarkdownDiffViewProps) {
  return (
    <Content className="sv-markdown">
      <Markdown
        rehypePlugins={[[() => (tree: Root) => markSegments(tree, segments)]]}
        remarkPlugins={[remarkGfm]}
      >
        {source}
      </Markdown>
    </Content>
  );
}
