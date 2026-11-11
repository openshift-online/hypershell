import { Content } from "@patternfly/react-core";
import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";

import {
  rehypeSpecViewer,
  type SpecMarkdownOptions,
} from "./rehype-spec-viewer";

interface MarkdownViewProps {
  source: string;
  options: SpecMarkdownOptions;
}

/** Renders Markdown as React elements with the viewer's annotations. */
export function MarkdownView({ source, options }: MarkdownViewProps) {
  return (
    <Content className="sv-markdown">
      <Markdown
        rehypePlugins={[[rehypeSpecViewer, options]]}
        remarkPlugins={[remarkGfm]}
      >
        {source}
      </Markdown>
    </Content>
  );
}
