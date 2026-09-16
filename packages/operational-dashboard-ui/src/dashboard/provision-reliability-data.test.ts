import { describe, expect, it } from "vitest";

import type { OperationalMetric } from "../application/dashboard-types";
import {
  getProvisionReliabilityStatusLevel,
  getProvisionSuccessColor,
  parseProvisionReliabilityStats,
} from "./provision-reliability-data";
import { STATUS_DONUT_COLORS } from "./status-donut-colors";

const provisionReliabilityMetric: OperationalMetric = {
  id: "provision-reliability",
  provisionOutcomes: {
    failureCount24h: "1",
    successCount24h: "9",
    successRatePercent: "90.0",
  },
  value: "90.0",
};

describe("provision reliability data", () => {
  it("parses provision outcome stats from the metric", () => {
    expect(parseProvisionReliabilityStats(provisionReliabilityMetric)).toEqual({
      failureCount: 1,
      successCount: 9,
      successRatePercent: 90,
    });
  });

  it("maps success-rate thresholds to status levels", () => {
    expect(getProvisionReliabilityStatusLevel(99)).toBe("healthy");
    expect(getProvisionReliabilityStatusLevel(98.9)).toBe("warning");
    expect(getProvisionReliabilityStatusLevel(95)).toBe("warning");
    expect(getProvisionReliabilityStatusLevel(94.9)).toBe("danger");
  });

  it("colors the success slice by threshold", () => {
    expect(getProvisionSuccessColor(99)).toBe(STATUS_DONUT_COLORS.healthy);
    expect(getProvisionSuccessColor(97)).toBe(STATUS_DONUT_COLORS.degraded);
    expect(getProvisionSuccessColor(90)).toBe(STATUS_DONUT_COLORS.failed);
  });
});
