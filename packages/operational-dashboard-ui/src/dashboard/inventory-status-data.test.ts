import { createIntl } from "react-intl";
import { describe, expect, it } from "vitest";

import { messages } from "../messages";
import { buildInventoryStatusData } from "./inventory-status-data";

const intl = createIntl({
  locale: "en",
  messages: {
    [messages.inventoryStatusUnknown.id]: "Unknown",
    [messages.statusDonutLegend.id]: "{status}: {count}",
  },
});

describe("buildInventoryStatusData", () => {
  it("suppresses the donut when more than five non-zero buckets exist", () => {
    const series = buildInventoryStatusData(intl, {
      a: 1,
      b: 2,
      c: 3,
      d: 4,
      e: 5,
      f: 6,
    });

    expect(series.data).toEqual([]);
  });

  it("localizes unknown status buckets", () => {
    const series = buildInventoryStatusData(intl, {
      Ready: 2,
      unknown: 1,
    });

    expect(series.data).toEqual([
      { x: "Ready", y: 2 },
      { x: "Unknown", y: 1 },
    ]);
  });
});
