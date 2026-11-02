import { describe, expect, it } from "vitest";

import { measureText, truncateToWidth } from "./measure-text";

// Under jsdom there is no real 2D canvas text metrics, so these exercise the
// non-DOM fallback path (FLT-CAP-04 verifies pixel fit by inspection, not here).

describe("measureText fallback", () => {
  it("grows with text length", () => {
    const short = measureText("ab", 10, 700);
    const long = measureText("abcdef", 10, 700);
    expect(long).toBeGreaterThan(short);
  });

  it("estimates a heavier weight as wider", () => {
    expect(measureText("sample", 10, 700)).toBeGreaterThan(
      measureText("sample", 10, 400),
    );
  });

  it("returns zero width for empty text", () => {
    expect(measureText("", 10, 700)).toBe(0);
  });
});

describe("truncateToWidth", () => {
  it("returns the text unchanged when it already fits", () => {
    const text = "ok";
    const width = measureText(text, 10, 400) + 10;
    expect(truncateToWidth(text, 10, 400, width)).toBe(text);
  });

  it("shortens with a trailing ellipsis so the result fits", () => {
    const text = "a very long environment name that will not fit";
    const maxWidth = measureText("a very", 10, 400);
    const out = truncateToWidth(text, 10, 400, maxWidth);
    expect(out.endsWith("...")).toBe(true);
    expect(out.length).toBeLessThan(text.length);
    expect(measureText(out, 10, 400)).toBeLessThanOrEqual(maxWidth);
  });

  it("falls back to a bare ellipsis when nothing fits", () => {
    expect(truncateToWidth("anything", 10, 400, 0)).toBe("...");
  });
});
