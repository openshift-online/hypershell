import type { Element, ElementContent, Root } from "hast";

/** Visible text of a HAST node, in document order. */
export function textOfNode(node: Root | Element | ElementContent): string {
  if (node.type === "text") return node.value;
  if (node.type === "element") {
    return node.children.map(textOfNode).join("");
  }
  return "";
}

export interface TransformTextOptions {
  /** True for elements whose text must remain untouched. */
  skipElement?: (element: Element) => boolean;
}

/**
 * Replaces text nodes with the nodes returned by `replace`. Elements selected
 * by `skipElement`, and their descendants, are left unchanged.
 */
export function transformTextNodes(
  node: Root | Element,
  replace: (text: string) => ElementContent[],
  options: TransformTextOptions = {},
): void {
  const visit = (parent: Root | Element, skipped: boolean): void => {
    const children: ElementContent[] = [];
    for (const child of parent.children) {
      if (child.type === "text") {
        children.push(...(skipped ? [child] : replace(child.value)));
        continue;
      }
      if (child.type === "element") {
        const childSkipped = skipped || Boolean(options.skipElement?.(child));
        visit(child, childSkipped);
        children.push(child);
        continue;
      }
    }
    parent.children = children;
  };
  visit(node, false);
}
