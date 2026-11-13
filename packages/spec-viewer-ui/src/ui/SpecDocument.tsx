import {
  EmptyState,
  EmptyStateBody,
  Title,
  Tooltip,
} from "@patternfly/react-core";
import { FileAltIcon } from "@patternfly/react-icons";
import { useEffect, useMemo, useRef, useState } from "react";
import { useIntl } from "react-intl";

import type { SpecCatalog, SpecFile, SpecNode } from "../domain/spec-document";
import { resolveSpecLink, resolveSpecReference } from "../domain/spec-links";
import { messages } from "../messages";
import { MarkdownView } from "./MarkdownView";
import { ID_LABEL_ATTRIBUTE } from "./rehype-spec-viewer";

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
  /** Lists the references to a requirement or scenario, by node id. */
  onFindReferences: (nodeId: string) => void;
}

/** The ID label under `target`, if any. */
function idLabelAt(target: EventTarget | null): HTMLElement | null {
  return target instanceof Element
    ? target.closest<HTMLElement>(`[${ID_LABEL_ATTRIBUTE}]`)
    : null;
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
  onFindReferences,
}: SpecDocumentProps) {
  const intl = useIntl();
  const scroller = useRef<HTMLDivElement>(null);
  // The ID label under the pointer or focus; one tooltip serves them all.
  const [hoveredLabel, setHoveredLabel] = useState<HTMLElement | null>(null);
  const nodesBySpecId = useMemo(() => {
    const byId = new Map<string, SpecNode>();
    for (const entry of catalog.nodesById.values()) {
      if (entry.specId && !byId.has(entry.specId))
        byId.set(entry.specId, entry);
    }
    return byId;
  }, [catalog]);
  // The same element across renders, so showing the ID tooltip never makes
  // React rewrite the document under the pointer.
  const markdownContent = useMemo(
    () =>
      file ? (
        <MarkdownView
          options={{
            headingId: headingElementId,
            highlightTerms,
            resolveId: (id) => nodesBySpecId.get(id),
            resolveLink: (href) => resolveSpecLink(catalog, file.path, href),
            resolveCode: (text) =>
              resolveSpecReference(catalog, file.path, text),
            unresolvedTitle: (path) =>
              intl.formatMessage(messages.unresolvedLink, { path }),
          }}
          source={file.content}
        />
      ) : null,
    [catalog, file, highlightTerms, intl, nodesBySpecId],
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
  }, [file, node, revision]);

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
        {/* Clicks, hovers and focus are handled for the ID labels inside. */}
        {/* eslint-disable-next-line jsx-a11y/click-events-have-key-events, jsx-a11y/no-static-element-interactions -- the labels are buttons, so keys reach them as clicks */}
        <div
          onClick={(event) => {
            const label = idLabelAt(event.target);
            const nodeId = label?.getAttribute(ID_LABEL_ATTRIBUTE);
            if (nodeId) onFindReferences(nodeId);
          }}
          onBlur={() => {
            setHoveredLabel(null);
          }}
          onFocus={(event) => {
            setHoveredLabel(idLabelAt(event.target));
          }}
          onMouseLeave={() => {
            setHoveredLabel(null);
          }}
          onMouseOver={(event) => {
            setHoveredLabel(idLabelAt(event.target));
          }}
        >
          {markdownContent}
        </div>
        {hoveredLabel ? (
          <Tooltip
            content={intl.formatMessage(messages.findReferences)}
            isVisible
            triggerRef={() => hoveredLabel}
          />
        ) : null}
      </div>
    </div>
  );
}
