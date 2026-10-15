import { describe, expect, it } from "vitest";

import { joinCookieChunks, splitCookieValue } from "../src/cookie-chunks.js";

describe("splitCookieValue", () => {
  it("keeps a short value in one chunk", () => {
    expect(splitCookieValue("abc", 10)).toEqual(["abc"]);
  });

  it("keeps an empty value as one empty chunk so deletions still clear it", () => {
    expect(splitCookieValue("", 10)).toEqual([""]);
  });

  it("budgets each chunk by its URL-encoded length", () => {
    // "+" and "/" encode to three bytes each, as base64 cookie values do.
    const value = "ab+/cd+/ef";
    const chunks = splitCookieValue(value, 6);
    expect(chunks.join("")).toBe(value);
    for (const chunk of chunks) {
      expect(encodeURIComponent(chunk).length).toBeLessThanOrEqual(6);
    }
  });
});

describe("joinCookieChunks", () => {
  const names = new Set(["session", "session_tok"]);

  it("leaves a header without continuation cookies unchanged", () => {
    const header = "session=abc; other=1";
    const result = joinCookieChunks(header, names);
    expect(result.header).toBe(header);
    expect(result.counts.get("session")).toBe(1);
  });

  it("joins continuation chunks in order and drops them from the header", () => {
    const result = joinCookieChunks(
      "other=1; session.2=ghi; session=abc; session.1=def; session_tok=xyz",
      names,
    );
    expect(result.header).toBe("other=1; session=abcdefghi; session_tok=xyz");
    expect(result.counts.get("session")).toBe(3);
    expect(result.counts.get("session_tok")).toBe(1);
  });

  it("stops at the first missing chunk index", () => {
    const result = joinCookieChunks("session=abc; session.2=ghi", names);
    expect(result.header).toBe("session=abc; session.2=ghi");
    expect(result.counts.get("session")).toBe(1);
  });

  it("round-trips a split value through URL-encoded cookies", () => {
    const value = "Zm9v+YmFy/YmF6=;bm9uY2U=".repeat(50);
    const chunks = splitCookieValue(value, 100);
    expect(chunks.length).toBeGreaterThan(1);
    const header = chunks
      .map(
        (chunk, index) =>
          `${index === 0 ? "session" : `session.${String(index)}`}=${encodeURIComponent(chunk)}`,
      )
      .join("; ");
    const joined = joinCookieChunks(header, names).header;
    expect(decodeURIComponent(joined.slice("session=".length))).toBe(value);
  });
});
