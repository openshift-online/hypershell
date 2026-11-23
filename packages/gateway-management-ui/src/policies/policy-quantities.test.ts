import { describe, expect, it } from "vitest";

import {
  additionalRules,
  fleetDefaultFor,
  formatCpuMillis,
  formatMemoryBytes,
  formatPolicyFieldValue,
  parseCount,
  parseCpuMillis,
  parseMemoryBytes,
  policyBoundSide,
  rowBounds,
} from "./policy-quantities";

describe("policy quantities", () => {
  it("converts CPU quantities to millicores", () => {
    expect(parseCpuMillis("250m")).toBe(250);
    expect(parseCpuMillis("4")).toBe(4000);
    expect(parseCpuMillis("0.5")).toBe(500);
    expect(parseCpuMillis(" 2 ")).toBe(2000);
    expect(parseCpuMillis("1k")).toBe(1_000_000);
    expect(parseCpuMillis("0.0001")).toBeUndefined();
    expect(parseCpuMillis("")).toBeUndefined();
    expect(parseCpuMillis("-1")).toBeUndefined();
    expect(parseCpuMillis("abc")).toBeUndefined();
  });

  it("converts memory quantities to bytes", () => {
    expect(parseMemoryBytes("512Mi")).toBe(536870912);
    expect(parseMemoryBytes("16Gi")).toBe(17179869184);
    expect(parseMemoryBytes("1G")).toBe(1_000_000_000);
    expect(parseMemoryBytes("1.5Ki")).toBe(1536);
    expect(parseMemoryBytes("100")).toBe(100);
    expect(parseMemoryBytes("1m")).toBeUndefined();
    expect(parseMemoryBytes("xGi")).toBeUndefined();
    expect(parseMemoryBytes("1Ei")).toBeUndefined();
  });

  it("parses counts and formats quantities like the API renderer", () => {
    expect(parseCount("2")).toBe(2);
    expect(parseCount("1.5")).toBeUndefined();
    expect(formatCpuMillis(250)).toBe("250m");
    expect(formatCpuMillis(4000)).toBe("4");
    expect(formatMemoryBytes(17179869184)).toBe("16Gi");
    expect(formatMemoryBytes(536870912)).toBe("512Mi");
    expect(formatMemoryBytes(2 * 1024 ** 4)).toBe("2Ti");
    expect(formatMemoryBytes(2048)).toBe("2Ki");
    expect(formatMemoryBytes(0)).toBe("0");
    expect(formatPolicyFieldValue("cpuLimitMillis", 8000)).toBe("8");
    expect(formatPolicyFieldValue("memoryRequestBytes", 1073741824)).toBe(
      "1Gi",
    );
    expect(formatPolicyFieldValue("gpuCount", 2)).toBe("2");
  });

  it("attributes recognized bounds to resource rows", () => {
    expect(policyBoundSide(">")).toBe("maximum");
    expect(policyBoundSide(">=")).toBe("maximum");
    expect(policyBoundSide("<")).toBe("minimum");
    expect(policyBoundSide("<=")).toBe("minimum");
    expect(policyBoundSide("==")).toBeUndefined();
    const bound = (field: string, op: string) => ({
      display: "1",
      field,
      label: field,
      op,
      text: field,
      value: 1,
    });
    const rules = [
      {
        bounds: [bound("cpuLimitMillis", ">")],
        cedar: "a",
        recognized: true,
        summary: [],
      },
      {
        bounds: [bound("gpuCount", "==")],
        cedar: "b",
        recognized: true,
        summary: [],
      },
      { bounds: [], cedar: "c", recognized: false, summary: [] },
      {
        bounds: [bound("image", ">")],
        cedar: "d",
        recognized: true,
        summary: [],
      },
    ];
    expect(rowBounds(rules)).toEqual([
      {
        bound: bound("cpuLimitMillis", ">"),
        resource: "cpuLimit",
        side: "maximum",
      },
    ]);
    expect(additionalRules(rules).map(({ cedar }) => cedar)).toEqual([
      "b",
      "c",
      "d",
    ]);
    expect(fleetDefaultFor({ cpuLimit: "2" }, "cpuLimit")).toBe("2");
    expect(fleetDefaultFor({ cpuLimit: "2" }, "gpuCount")).toBeUndefined();
  });
});
