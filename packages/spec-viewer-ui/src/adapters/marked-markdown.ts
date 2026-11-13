import { Lexer, type MarkedToken, type Token, type Tokens } from "marked";

import type { MarkdownPort } from "../application/ports";
import type { MarkdownBlock } from "../domain/spec-document";

const MARKED_TOKEN_TYPES = new Set([
  "blockquote",
  "br",
  "checkbox",
  "code",
  "codespan",
  "def",
  "del",
  "em",
  "escape",
  "heading",
  "hr",
  "html",
  "image",
  "link",
  "list",
  "list_item",
  "paragraph",
  "space",
  "strong",
  "table",
  "tag",
  "text",
]);

function isMarkedToken(token: Token): token is MarkedToken {
  return MARKED_TOKEN_TYPES.has(token.type);
}

function tokenText(token: MarkedToken): string {
  if (token.type === "html") return "";
  if (token.type === "code") return token.text;
  if (token.type === "table") {
    return [
      ...token.header.map((cell: Tokens.TableCell) =>
        cell.tokens
          .map((nested) => (isMarkedToken(nested) ? tokenText(nested) : ""))
          .join(""),
      ),
      ...token.rows.flatMap((row: Tokens.TableCell[]) =>
        row.map((cell) =>
          cell.tokens
            .map((nested) => (isMarkedToken(nested) ? tokenText(nested) : ""))
            .join(""),
        ),
      ),
    ].join("");
  }
  if (token.type === "list") {
    return token.items.map(tokenText).join("");
  }
  if ("tokens" in token && Array.isArray(token.tokens)) {
    return token.tokens
      .map((nested) => (isMarkedToken(nested) ? tokenText(nested) : ""))
      .join("");
  }
  return "text" in token ? token.text : "";
}

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
  text(source) {
    return Lexer.lex(source, { gfm: true })
      .map((token) => (isMarkedToken(token) ? tokenText(token) : ""))
      .join("");
  },
};
