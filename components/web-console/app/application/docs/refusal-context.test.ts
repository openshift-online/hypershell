import { describe, expect, it } from "vitest";

import { readRefusalContext } from "./refusal-context";

describe("readRefusalContext", () => {
  // Verifies: WEB-UDOC-09, WEB-UDOC-09.01
  it("keeps valid policy and gateway values", () => {
    expect(
      readRefusalContext(
        new URLSearchParams("policy=sandbox-cpu-limit&gateway=team-a"),
      ),
    ).toEqual({ gateway: "team-a", policy: "sandbox-cpu-limit" });
  });

  // Verifies: WEB-UDOC-09.02
  it.each([
    "<script>x</script>",
    "",
    "a".repeat(129),
    "name with spaces",
    "naïve",
    "rm;-rf",
    "$(id)",
  ])("ignores the invalid value %j", (value) => {
    const params = new URLSearchParams();
    params.set("policy", value);
    params.set("gateway", value);
    expect(readRefusalContext(params)).toEqual({});
  });

  it("accepts the 128-character limit and the allowed punctuation", () => {
    const value = `a.b_c-${"d".repeat(122)}`;
    expect(value).toHaveLength(128);
    const params = new URLSearchParams();
    params.set("policy", value);
    expect(readRefusalContext(params)).toEqual({ policy: value });
  });
});
