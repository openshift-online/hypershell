import { describe, expect, it } from "vitest";

import { shortDigest } from "./digest";

describe("shortDigest", () => {
  it("abbreviates a sha256 digest to 7 chars each side, keeping the prefix", () => {
    const full =
      "sha256:6145e7f19d28d502b57f21aac8a60d4b07049390810264078e9bbc3e7aebd608";
    expect(shortDigest(full)).toBe("sha256:6145e7f...aebd608");
  });

  it("truncates a bare hash with no algorithm prefix", () => {
    const full =
      "6145e7f19d28d502b57f21aac8a60d4b07049390810264078e9bbc3e7aebd608";
    expect(shortDigest(full)).toBe("6145e7f...aebd608");
  });

  it("leaves a value already short enough untouched", () => {
    // The short-SHA fallback identity (10 chars) is shorter than 7+3+7.
    expect(shortDigest("c2bdf68dc5")).toBe("c2bdf68dc5");
    expect(shortDigest("sha256:abcdef")).toBe("sha256:abcdef");
  });

  it("returns empty string for empty / nullish input", () => {
    expect(shortDigest("")).toBe("");
    expect(shortDigest(null)).toBe("");
    expect(shortDigest(undefined)).toBe("");
  });
});
