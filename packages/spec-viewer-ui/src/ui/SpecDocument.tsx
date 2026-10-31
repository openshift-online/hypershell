import {
  Content,
  EmptyState,
  EmptyStateBody,
  Title,
} from "@patternfly/react-core";
import { FileAltIcon } from "@patternfly/react-icons";
import { useEffect, useMemo, useRef } from "react";
import { useIntl } from "react-intl";

import type { SpecCatalog, SpecFile, SpecNode } from "../domain/spec-document";
import { resolveSpecLink, resolveSpecReference } from "../domain/spec-links";
import { messages } from "../messages";
import { highlightHtml } from "./highlight-html";
import { rewriteLinks } from "./link-html";
import { usePorts } from "./ports-context";

export const FLASH_CLASS = "sv-flash";

export function headingElementId(index: number): string {
  return `spec-heading-${String(index)}`;
}

interface SpecDocumentProps {
  catalog: SpecCatalog;
  file: SpecFile | undefined;
  node: SpecNode | undefined;
  revision: number;
  /** Search text to mark in the document. */
  highlightTerms: string[];
}

/**
 * Renders one spec file, scrolls to the selected heading, and flashes it.
 * Spec files are local, trusted repository content.
 */
export function SpecDocument({
  catalog,
  file,
  node,
  revision,
  highlightTerms,
}: SpecDocumentProps) {
  const intl = useIntl();
  const { markdown } = usePorts();
  const scroller = useRef<HTMLDivElement>(null);
  const html = useMemo(() => {
    if (!file) return "";
    return rewriteLinks(markdown.render(file.content, headingElementId), {
      href: (href) => resolveSpecLink(catalog, file.path, href),
      code: (text) => resolveSpecReference(catalog, file.path, text),
      unresolvedTitle: (path) =>
        intl.formatMessage(messages.unresolvedLink, { path }),
    });
  }, [catalog, file, markdown, intl]);
  const highlightedHtml = useMemo(
    () => highlightHtml(html, highlightTerms),
    [html, highlightTerms],
  );

  useEffect(() => {
    const container = scroller.current;
    if (!container || !node) return;
    if (node.headingIndex === null) {
      container.scrollTo({ top: 0 });
      return;
    }
    const heading = container.querySelector<HTMLElement>(
      `#${headingElementId(node.headingIndex)}`,
    );
    if (!heading) return;
    heading.scrollIntoView({ block: "start" });
    heading.focus({ preventScroll: true });
    // Restart the animation when the same heading is selected again.
    heading.classList.remove(FLASH_CLASS);
    void heading.offsetWidth;
    heading.classList.add(FLASH_CLASS);
    const clear = () => {
      heading.classList.remove(FLASH_CLASS);
    };
    heading.addEventListener("animationend", clear, { once: true });
    return () => {
      heading.removeEventListener("animationend", clear);
    };
  }, [html, node, revision]);

  if (!file) {
    return (
      <EmptyState
        headingLevel="h2"
        icon={FileAltIcon}
        titleText={intl.formatMessage(messages.documentEmptyTitle)}
      >
        <EmptyStateBody>
          {intl.formatMessage(messages.documentEmptyBody)}
        </EmptyStateBody>
      </EmptyState>
    );
  }

  return (
    <div className="sv-document">
      <div className="sv-document__header">
        <Title headingLevel="h2" size="md">
          {file.path}
        </Title>
      </div>
      <div
        aria-label={intl.formatMessage(messages.documentLabel)}
        className="sv-document__body"
        ref={scroller}
        role="region"
        tabIndex={-1}
      >
        <Content
          className="sv-markdown"
          dangerouslySetInnerHTML={{ __html: highlightedHtml }}
        />
      </div>
    </div>
  );
}
