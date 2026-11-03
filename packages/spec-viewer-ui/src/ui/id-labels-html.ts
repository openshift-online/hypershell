import type { SpecNode } from "../domain/spec-document";

/** Attribute holding the node id of a document ID label. */
export const ID_LABEL_ATTRIBUTE = "data-node-id";

/**
 * A word shaped like a requirement or scenario ID: QRX-01, ZZ-A11Y-01,
 * QRX-01.02. The lookbehind keeps it from starting inside a longer word.
 */
const ID_TOKEN = /(?<![A-Za-z0-9_.-])[A-Z][A-Z0-9]*(?:[-.][A-Z0-9]+)+/gu;

/** Text inside these elements stays as it is. */
const SKIP = "a, pre, button, .pf-v6-c-label";

/**
 * Builds the PatternFly compact label markup that `NodeLabels` renders, so
 * IDs in the document look like the ones in the table.
 */
function idLabel(node: SpecNode, id: string): HTMLElement {
  const label = document.createElement("span");
  label.className =
    "pf-v6-c-label pf-m-blue pf-m-filled pf-m-compact pf-m-clickable sv-doc-id";
  if (node.kind === "spec") label.classList.add("sv-id-label--requirement");
  const button = document.createElement("button");
  button.className = "pf-v6-c-label__content pf-m-clickable";
  button.type = "button";
  button.setAttribute(ID_LABEL_ATTRIBUTE, node.id);
  const text = document.createElement("span");
  text.className = "pf-v6-c-label__text";
  text.textContent = id;
  button.append(text);
  label.append(button);
  return label;
}

/**
 * Turns every requirement or scenario ID in the text of `html` into a
 * clickable label. Only IDs that `resolve` knows become labels, so words
 * that merely look like IDs stay plain text. Inline code that holds just an
 * ID is replaced by the label.
 */
export function labelIds(
  html: string,
  resolve: (id: string) => SpecNode | undefined,
): string {
  if (html === "") return html;
  const template = document.createElement("template");
  template.innerHTML = html;
  const walker = document.createTreeWalker(
    template.content,
    NodeFilter.SHOW_TEXT,
  );
  const textNodes: Text[] = [];
  while (walker.nextNode()) textNodes.push(walker.currentNode as Text);

  for (const textNode of textNodes) {
    if (textNode.parentElement?.closest(SKIP)) continue;
    const text = textNode.data;
    const parts: (string | HTMLElement)[] = [];
    let last = 0;
    for (const match of text.matchAll(ID_TOKEN)) {
      const node = resolve(match[0]);
      if (!node) continue;
      parts.push(text.slice(last, match.index), idLabel(node, match[0]));
      last = match.index + match[0].length;
    }
    if (parts.length === 0) continue;
    parts.push(text.slice(last));
    const code = textNode.parentElement;
    if (
      code?.tagName === "CODE" &&
      code.childNodes.length === 1 &&
      parts.length === 3 &&
      parts[0] === "" &&
      parts[2] === ""
    ) {
      code.replaceWith(parts[1] as HTMLElement);
    } else {
      textNode.replaceWith(...parts);
    }
  }
  return template.innerHTML;
}
