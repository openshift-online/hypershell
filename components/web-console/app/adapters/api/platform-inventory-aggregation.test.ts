import { describe, expect, it } from "vitest";

import {
  bucketInventoryField,
  countCreatedWithinLookback,
  formatInventoryPlacementLabel,
} from "./platform-inventory-aggregation";

describe("platform inventory aggregation helpers", () => {
  it("buckets omitted and blank fields as unknown", () => {
    expect(bucketInventoryField(undefined)).toBe("unknown");
    expect(bucketInventoryField(null)).toBe("unknown");
    expect(bucketInventoryField("  ")).toBe("unknown");
    expect(bucketInventoryField(" Ready ")).toBe("Ready");
  });

  it("formats placement labels as region (provider)", () => {
    expect(formatInventoryPlacementLabel("us-east-1", "aws")).toBe(
      "us-east-1 (aws)",
    );
    expect(formatInventoryPlacementLabel("  ", "openshift")).toBe(
      "unknown (openshift)",
    );
    expect(formatInventoryPlacementLabel(undefined, undefined)).toBe(
      "unknown (unknown)",
    );
  });

  it("counts items created within the 30-day lookback window", () => {
    const evaluationTime = new Date("2026-09-01T12:00:00.000Z");

    expect(
      countCreatedWithinLookback("2026-08-15T10:00:00.000Z", evaluationTime),
    ).toBe(true);
    expect(
      countCreatedWithinLookback("2026-07-01T10:00:00.000Z", evaluationTime),
    ).toBe(false);
    expect(countCreatedWithinLookback(undefined, evaluationTime)).toBe(false);
    expect(countCreatedWithinLookback("not-a-date", evaluationTime)).toBe(
      false,
    );
  });
});
