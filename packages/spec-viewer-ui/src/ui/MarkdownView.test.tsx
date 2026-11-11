import { render, screen } from "@testing-library/react";
import { IntlProvider } from "react-intl";

import { markedMarkdown } from "../adapters/marked-markdown";
import { buildCatalog, parseSpecFile } from "../domain/spec-document";
import { resolveSpecLink, resolveSpecReference } from "../domain/spec-links";
import { MarkdownView } from "./MarkdownView";

const source = `# Security

<script>alert(1)</script>

[bad](javascript:alert(1))
`;

const file = parseSpecFile("security.spec.md", markedMarkdown.lex(source));
const catalog = buildCatalog([file]);

describe("MarkdownView", () => {
  it("does not render raw HTML or unsafe URLs", () => {
    render(
      <IntlProvider defaultLocale="en" locale="en">
        <MarkdownView
          options={{
            headingId: (index) => `heading-${String(index)}`,
            highlightTerms: [],
            resolveId: () => undefined,
            resolveLink: (href) => resolveSpecLink(catalog, file.path, href),
            resolveCode: (text) =>
              resolveSpecReference(catalog, file.path, text),
            unresolvedTitle: (path) => `Unresolved: ${path}`,
          }}
          source={source}
        />
      </IntlProvider>,
    );

    expect(document.querySelector("script")).toBeNull();
    const link = screen.getByText("bad").closest("a");
    if (!link) throw new Error("Missing unsafe link");
    expect(link.getAttribute("href")).not.toContain("javascript:");
  });
});
