import { highlightHtml, MATCH_CLASS } from "./highlight-html";

describe("highlightHtml", () => {
  it("marks terms in text, ignoring case, without touching tags", () => {
    expect(
      highlightHtml(
        '<p class="secret">A Secret and a <code>secret</code>.</p>',
        ["secret"],
      ),
    ).toBe(
      `<p class="secret">A <mark class="${MATCH_CLASS}">Secret</mark> and a <code><mark class="${MATCH_CLASS}">secret</mark></code>.</p>`,
    );
  });

  it("prefers the longest term and lets phrases span whitespace", () => {
    expect(
      highlightHtml("<p>client\nsecret client</p>", [
        "client",
        "client secret",
      ]),
    ).toBe(
      `<p><mark class="${MATCH_CLASS}">client\nsecret</mark> <mark class="${MATCH_CLASS}">client</mark></p>`,
    );
  });

  it("escapes regular expression characters and skips one-letter terms", () => {
    expect(highlightHtml("<p>a (b) c+</p>", ["(b)", "c+", "a"])).toBe(
      `<p>a <mark class="${MATCH_CLASS}">(b)</mark> <mark class="${MATCH_CLASS}">c+</mark></p>`,
    );
    expect(highlightHtml("<p>x</p>", [])).toBe("<p>x</p>");
  });
});
