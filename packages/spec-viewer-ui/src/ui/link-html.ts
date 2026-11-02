import type { LinkTarget } from "../domain/spec-links";

export const SPEC_LINK_CLASS = "sv-spec-link";
export const UNRESOLVED_LINK_CLASS = "sv-link--unresolved";

export interface LinkResolvers {
  /** Resolves an `<a href>` written in the document. */
  href: (href: string) => LinkTarget;
  /** Resolves inline code that may name a spec file; null when it does not. */
  code: (text: string) => string | null;
  /** Tooltip for a relative link that points outside the specs folder. */
  unresolvedTitle: (path: string) => string;
}

/** Fragment that the viewer's selection reads (see use-selection). */
export function nodeHref(nodeId: string): string {
  return `#${encodeURIComponent(nodeId)}`;
}

/**
 * Points links at other specs to their node in the viewer, opens external
 * links in a new tab, and disables relative links that leave the specs
 * folder. Inline code naming a spec file becomes a link as well.
 */
export function rewriteLinks(html: string, resolvers: LinkResolvers): string {
  if (html === "") return html;
  const template = document.createElement("template");
  template.innerHTML = html;
  const content = template.content;

  for (const anchor of content.querySelectorAll("a[href]")) {
    const href = anchor.getAttribute("href") ?? "";
    const target = resolvers.href(href);
    if (target.kind === "spec") {
      anchor.setAttribute("href", nodeHref(target.nodeId));
      anchor.classList.add(SPEC_LINK_CLASS);
    } else if (target.kind === "external") {
      anchor.setAttribute("target", "_blank");
      anchor.setAttribute("rel", "noopener noreferrer");
    } else {
      anchor.removeAttribute("href");
      anchor.classList.add(UNRESOLVED_LINK_CLASS);
      anchor.setAttribute("title", resolvers.unresolvedTitle(target.path));
    }
  }

  for (const code of content.querySelectorAll("code")) {
    if (code.closest("a, pre")) continue;
    const nodeId = resolvers.code(code.textContent);
    if (!nodeId) continue;
    const link = document.createElement("a");
    link.setAttribute("href", nodeHref(nodeId));
    link.className = SPEC_LINK_CLASS;
    code.replaceWith(link);
    link.append(code);
  }
  return template.innerHTML;
}
