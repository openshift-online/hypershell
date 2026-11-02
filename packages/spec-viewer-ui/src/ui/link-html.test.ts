import {
  nodeHref,
  rewriteLinks,
  SPEC_LINK_CLASS,
  UNRESOLVED_LINK_CLASS,
} from "./link-html";

const resolvers = {
  href: (href: string) =>
    href.startsWith("http")
      ? ({ kind: "external" } as const)
      : href.endsWith(".spec.md")
        ? ({ kind: "spec", nodeId: `${href}#top` } as const)
        : ({ kind: "unresolved", path: href } as const),
  code: (text: string) => (text.endsWith(".spec.md") ? `${text}#top` : null),
  unresolvedTitle: (path: string) => `outside: ${path}`,
};

function render(html: string): HTMLElement {
  const container = document.createElement("div");
  container.innerHTML = rewriteLinks(html, resolvers);
  return container;
}

describe("rewriteLinks", () => {
  it("points spec links at viewer nodes", () => {
    const link = render('<a href="a.spec.md">A</a>').querySelector("a");
    expect(link?.getAttribute("href")).toBe(nodeHref("a.spec.md#top"));
    expect(link?.classList.contains(SPEC_LINK_CLASS)).toBe(true);
  });

  it("opens external links in a new tab", () => {
    const link = render('<a href="https://x.dev">X</a>').querySelector("a");
    expect(link?.getAttribute("target")).toBe("_blank");
    expect(link?.getAttribute("rel")).toBe("noopener noreferrer");
  });

  it("disables links outside the specs folder", () => {
    const link = render('<a href="../../x.ref">X</a>').querySelector("a");
    expect(link?.hasAttribute("href")).toBe(false);
    expect(link?.classList.contains(UNRESOLVED_LINK_CLASS)).toBe(true);
    expect(link?.title).toBe("outside: ../../x.ref");
  });

  it("links inline code that names a spec, but not code in blocks or links", () => {
    const container = render(
      '<p><code>b.spec.md</code> <code>make</code></p><pre><code>c.spec.md</code></pre><a href="https://x.dev"><code>d.spec.md</code></a>',
    );
    const links = [...container.querySelectorAll("a")].map((a) => [
      a.getAttribute("href"),
      a.textContent,
    ]);
    expect(links).toEqual([
      [nodeHref("b.spec.md#top"), "b.spec.md"],
      ["https://x.dev", "d.spec.md"],
    ]);
  });

  it("leaves empty documents alone", () => {
    expect(rewriteLinks("", resolvers)).toBe("");
  });
});
