import { describe, expect, it } from "vitest";

import { planeFreshness } from "./plane";

const AGING = 60_000;
const STALE = 300_000;

describe("planeFreshness", () => {
  const base = Date.parse("2026-01-01T00:00:00Z");

  it("is fresh within the aging window", () => {
    expect(
      planeFreshness(
        { generatedAt: "2026-01-01T00:00:00Z", stale: false },
        base + 30_000,
        AGING,
        STALE,
      ),
    ).toBe("fresh");
  });

  it("is aging past the aging threshold", () => {
    expect(
      planeFreshness(
        { generatedAt: "2026-01-01T00:00:00Z", stale: false },
        base + 120_000,
        AGING,
        STALE,
      ),
    ).toBe("aging");
  });

  it("is stale past the stale threshold", () => {
    expect(
      planeFreshness(
        { generatedAt: "2026-01-01T00:00:00Z", stale: false },
        base + 400_000,
        AGING,
        STALE,
      ),
    ).toBe("stale");
  });

  it("is stale when the server flags a last-good value regardless of age", () => {
    expect(
      planeFreshness(
        { generatedAt: "2026-01-01T00:00:00Z", stale: true },
        base + 1,
        AGING,
        STALE,
      ),
    ).toBe("stale");
  });

  it("is stale when the timestamp is unparseable", () => {
    expect(
      planeFreshness(
        { generatedAt: "not-a-date", stale: false },
        base,
        AGING,
        STALE,
      ),
    ).toBe("stale");
  });
});
