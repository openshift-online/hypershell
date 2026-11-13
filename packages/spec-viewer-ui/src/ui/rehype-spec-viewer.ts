import type { Element, ElementContent, Properties, Root } from "hast";

import type { LinkTarget } from "../domain/spec-links";
import type { SpecNode } from "../domain/spec-document";
import { textOfNode, transformTextNodes } from "./hast-utils";
import { highlightHast } from "./highlight-hast";

export const ID_LABEL_ATTRIBUTE = "data-node-id";
export const SPEC_LINK_CLASS = "sv-spec-link";
export const UNRESOLVED_LINK_CLASS = "sv-link--unresolved";

/** A word shaped like a requirement or scenario ID. */
const ID_TOKEN = /(?<![A-Za-z0-9_.-])[A-Z][A-Z0-9]*(?:[-.][A-Z0-9]+)+/gu;

const SKIP_TEXT = new Set(["a", "pre", "button"]);

export interface SpecMarkdownOptions {
  /** The n-th heading receives the id returned by this function. */
  headingId: (index: number) => string;
  /** Search terms to mark in the document. */
  highlightTerms: string[];
  /** Resolves a requirement or scenario ID to its catalog node. */
  resolveId: (id: string) => SpecNode | undefined;
  /** Resolves an href written in the document. */
  resolveLink: (href: string) => LinkTarget;
  /** Resolves inline code that names a spec file to a node id. */
  resolveCode: (text: string) => string | null;
  /** Tooltip for a relative link outside the specs folder. */
  unresolvedTitle: (path: string) => string;
}

/** Fragment that the viewer's selection reads (see use-selection). */
export function nodeHref(nodeId: string): string {
  return `#${encodeURIComponent(nodeId)}`;
}

function element(
  tagName: string,
  properties: Properties,
  children: ElementContent[],
): Element {
  return {
    type: "element",
    tagName,
    properties,
    children,
  };
}

function text(value: string): ElementContent {
  return { type: "text", value };
}

function idLabel(node: SpecNode, id: string): Element {
  const label = element(
    "span",
    {
      className: [
        "pf-v6-c-label",
        "pf-m-blue",
        "pf-m-filled",
        "pf-m-compact",
        "pf-m-clickable",
        "sv-doc-id",
        ...(node.kind === "spec" ? ["sv-id-label--requirement"] : []),
      ],
    },
    [],
  );
  const button = element(
    "button",
    {
      className: ["pf-v6-c-label__content", "pf-m-clickable"],
      type: "button",
      dataNodeId: node.id,
    },
    [],
  );
  const labelContent = element("span", { className: ["pf-v6-c-label__text"] }, [
    text(id),
  ]);
  button.children = [labelContent];
  label.children = [button];
  return label;
}

function labelIds(root: Root, resolve: (id: string) => SpecNode | undefined) {
  transformTextNodes(
    root,
    (value) => {
      const parts: ElementContent[] = [];
      let last = 0;
      for (const match of value.matchAll(ID_TOKEN)) {
        const node = resolve(match[0]);
        if (!node) continue;
        parts.push(text(value.slice(last, match.index)));
        parts.push(idLabel(node, match[0]));
        last = match.index + match[0].length;
      }
      if (parts.length === 0) return [text(value)];
      parts.push(text(value.slice(last)));
      return parts;
    },
    { skipElement: (element) => SKIP_TEXT.has(element.tagName) },
  );
}

function rewriteAnchor(anchor: Element, options: SpecMarkdownOptions): void {
  const href =
    typeof anchor.properties.href === "string" ? anchor.properties.href : "";
  const target = options.resolveLink(href);
  if (target.kind === "spec") {
    anchor.properties.href = nodeHref(target.nodeId);
    anchor.properties.className = [
      ...(anchor.properties.className ?? []),
      SPEC_LINK_CLASS,
    ];
  } else if (target.kind === "external") {
    anchor.properties.target = "_blank";
    anchor.properties.rel = ["noopener", "noreferrer"];
  } else {
    delete anchor.properties.href;
    anchor.properties.className = [
      ...(anchor.properties.className ?? []),
      UNRESOLVED_LINK_CLASS,
    ];
    anchor.properties.title = options.unresolvedTitle(target.path);
  }
}

function replaceCodeWithLink(code: Element, nodeId: string): Element {
  const link = element(
    "a",
    { href: nodeHref(nodeId), className: [SPEC_LINK_CLASS] },
    [code],
  );
  return link;
}

function rewriteContent(root: Root, options: SpecMarkdownOptions): void {
  let headingIndex = 0;
  const visit = (parent: Root | Element): void => {
    parent.children.forEach((child, index) => {
      if (child.type !== "element") {
        return;
      }
      if (/^h[1-6]$/u.test(child.tagName)) {
        child.properties.id = options.headingId(headingIndex);
        child.properties.tabIndex = -1;
        headingIndex += 1;
      }
      if (child.tagName === "a" && child.properties.href !== undefined) {
        rewriteAnchor(child, options);
      }
      visit(child);
      if (
        child.tagName === "code" &&
        !(parent.type === "element" && parent.tagName === "pre") &&
        !(parent.type === "element" && parent.tagName === "a") &&
        !child.children.some((item) => item.type === "element")
      ) {
        const nodeId = options.resolveCode(textOfNode(child));
        if (nodeId) {
          parent.children[index] = replaceCodeWithLink(child, nodeId);
        }
      }
    });
  };
  visit(root);
}

/** Assigns sequential ids to headings without changing any other content. */
export function assignHeadingIds(
  root: Root,
  headingId: (index: number) => string,
): Root {
  let index = 0;
  const visit = (parent: Root | Element): void => {
    for (const child of parent.children) {
      if (child.type !== "element") continue;
      if (/^h[1-6]$/u.test(child.tagName)) {
        child.properties.id = headingId(index);
        child.properties.tabIndex = -1;
        index += 1;
      }
      visit(child);
    }
  };
  visit(root);
  return root;
}

/** Applies the viewer's links, labels, heading ids, and search marks. */
export function rehypeSpecViewer(options: SpecMarkdownOptions) {
  return (tree: Root): Root => {
    rewriteContent(tree, options);
    labelIds(tree, options.resolveId);
    return highlightHast(tree, options.highlightTerms);
  };
}
