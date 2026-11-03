export const MATCH_CLASS = "sv-match";

const escapeRegExp = (text: string): string =>
  text.replace(/[.*+?^${}()|[\]\\]/gu, "\\$&");

/**
 * Wraps every case-insensitive occurrence of `terms` in the text of `html`
 * with `<mark class="sv-match">`. Tags and attributes are left untouched.
 * Terms shorter than two characters are ignored as too noisy.
 */
export function highlightHtml(html: string, terms: string[]): string {
  const usable = terms
    .map((term) => term.trim())
    .filter((term) => term.length >= 2)
    .sort((a, b) => b.length - a.length);
  if (html === "" || usable.length === 0) return html;
  const pattern = new RegExp(
    usable.map((term) => escapeRegExp(term).replace(/ /gu, "\\s+")).join("|"),
    "giu",
  );

  const template = document.createElement("template");
  template.innerHTML = html;
  const walker = document.createTreeWalker(
    template.content,
    NodeFilter.SHOW_TEXT,
  );
  const textNodes: Text[] = [];
  while (walker.nextNode()) textNodes.push(walker.currentNode as Text);

  for (const node of textNodes) {
    const text = node.data;
    const parts: (string | HTMLElement)[] = [];
    let last = 0;
    for (const match of text.matchAll(pattern)) {
      parts.push(text.slice(last, match.index));
      const mark = document.createElement("mark");
      mark.className = MATCH_CLASS;
      mark.textContent = match[0];
      parts.push(mark);
      last = match.index + match[0].length;
    }
    if (parts.length === 0) continue;
    parts.push(text.slice(last));
    node.replaceWith(...parts);
  }
  return template.innerHTML;
}
