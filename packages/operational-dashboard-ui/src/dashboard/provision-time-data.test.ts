import { describe, expect, it } from "vitest";

import type { OperationalMetric } from "../application/dashboard-types";
import { parseProvisionDurationStats } from "./provision-time-data";

const provisionTimeMetric: OperationalMetric = {
  id: "provision-time",
  provisionDuration: {
    mean: "5.25",
    p50: "4.80",
    p95: "12.10",
  },
  unit: "minutes",
  value: "5.25",
};

describe("parseProvisionDurationStats", () => {
  it("parses mean, P50, and P95 from provisionDuration", () => {
    expect(parseProvisionDurationStats(provisionTimeMetric)).toEqual({
      meanMinutes: 5.25,
      p50Minutes: 4.8,
      p95Minutes: 12.1,
    });
  });

  it("returns undefined when any percentile is missing", () => {
    expect(
      parseProvisionDurationStats({
        ...provisionTimeMetric,
        provisionDuration: {
          mean: "5.25",
          p50: "4.80",
          p95: "NaN",
        },
      }),
    ).toBeUndefined();
  });
});
