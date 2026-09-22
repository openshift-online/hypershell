import { describe, expect, it } from "vitest";

import type { OperationalMetric } from "../application/dashboard-types";
import { parseProvisionDurationStats } from "./provision-time-data";

const provisionTimeMetric: OperationalMetric = {
  id: "provision-time",
  provisionDuration: {
    mean: "315.00",
    p50: "288.00",
    p95: "726.00",
  },
  unit: "sec",
  value: "315.00",
};

describe("parseProvisionDurationStats", () => {
  it("parses mean, P50, and P95 from provisionDuration", () => {
    expect(parseProvisionDurationStats(provisionTimeMetric)).toEqual({
      meanSeconds: 315,
      p50Seconds: 288,
      p95Seconds: 726,
    });
  });

  it("returns undefined when any percentile is missing", () => {
    expect(
      parseProvisionDurationStats({
        ...provisionTimeMetric,
        provisionDuration: {
          mean: "315.00",
          p50: "288.00",
          p95: "NaN",
        },
      }),
    ).toBeUndefined();
  });
});
