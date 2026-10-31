import { Lexer, Parser, type Token, type Tokens } from "marked";

import type { MarkdownPort } from "../application/ports";
import type { MarkdownBlock } from "../domain/spec-document";

const escapeAttribute = (value: string): string =>
  value.replace(/&/gu, "&amp;").replace(/"/gu, "&quot;");

export const markedMarkdown: MarkdownPort = {
  lex(source) {
    return Lexer.lex(source, { gfm: true }).map((token): MarkdownBlock =>
      token.type === "heading"
        ? {
            kind: "heading",
            depth: (token as Tokens.Heading).depth,
            text: (token as Tokens.Heading).text,
            raw: token.raw,
          }
        : { kind: "other", raw: token.raw },
    );
  },
  render(source, headingId) {
    let index = 0;
    const tokens = Lexer.lex(source, { gfm: true }).map((token): Token => {
      if (token.type !== "heading") return token;
      const heading = token as Tokens.Heading;
      const id = escapeAttribute(headingId(index));
      index += 1;
      const inner = Parser.parseInline(heading.tokens);
      return {
        type: "html",
        raw: heading.raw,
        pre: false,
        block: true,
        text: `<h${String(heading.depth)} id="${id}" tabindex="-1">${inner}</h${String(heading.depth)}>\n`,
      };
    });
    return Parser.parse(tokens, { gfm: true });
  },
};
