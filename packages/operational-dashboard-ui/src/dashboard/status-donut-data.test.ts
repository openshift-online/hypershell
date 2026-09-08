import { describe, expect, it } from "vitest";

import { buildStatusDonutData } from "./status-donut-data";

describe("buildStatusDonutData", () => {
  it("omits zero-count entries", () => {
    const result = buildStatusDonutData([
      {
        color: "#63993d",
        count: 5,
        label: "Ready",
        legendName: "Ready: 5",
      },
      {
        color: "#b1380b",
        count: 0,
        label: "Not ready",
        legendName: "Not ready: 0",
      },
    ]);

    expect(result).toEqual({
      colorScale: ["#63993d"],
      data: [{ x: "Ready", y: 5 }],
      legendData: [{ name: "Ready: 5" }],
    });
  });

  it("returns empty series when every entry is zero", () => {
    const result = buildStatusDonutData([
      {
        color: "#63993d",
        count: 0,
        label: "Ready",
        legendName: "Ready: 0",
      },
    ]);

    expect(result).toEqual({
      colorScale: [],
      data: [],
      legendData: [],
    });
  });
});
