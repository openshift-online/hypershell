import { markChangedWords } from "./diff-html";

describe("markChangedWords", () => {
  it("marks the words that differ and keeps the markup", () => {
    const [before, after] = markChangedWords(
      "<p>The gateway <strong>SHALL</strong> rotate daily</p>",
      "<p>The gateway <strong>MUST</strong> rotate hourly</p>",
    );
    expect(before).toBe(
      '<p>The gateway <strong><mark class="sv-diff__word">SHALL</mark></strong> rotate <mark class="sv-diff__word">daily</mark></p>',
    );
    expect(after).toContain(
      '<strong><mark class="sv-diff__word">MUST</mark></strong>',
    );
    expect(after).toContain('<mark class="sv-diff__word">hourly</mark>');
  });

  it("leaves identical blocks unmarked", () => {
    const html = "<p>Same <code>text</code></p>";
    expect(markChangedWords(html, html)).toEqual([html, html]);
  });
});
