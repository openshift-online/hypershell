import { createIntl } from "react-intl";
import { describe, expect, it } from "vitest";

import { messages } from "../messages";
import {
  buildInventoryDimensionDonutData,
  sortInventoryDimensionEntries,
} from "./inventory-dimension-donut-data";

const intl = createIntl({
  locale: "en",
  messages: {
    [messages.inventoryStatusUnknown.id]: "Unknown",
    [messages.statusDonutLegend.id]: "{status}: {count}",
  },
});

describe("inventory dimension donut data", () => {
  it("sorts dimensions by descending count then ascending label", () => {
    expect(
      sortInventoryDimensionEntries({
        aws: 5,
        gcp: 5,
        ibm: 2,
        openshift: 1,
      }),
    ).toEqual([
      { count: 5, label: "aws" },
      { count: 5, label: "gcp" },
      { count: 2, label: "ibm" },
      { count: 1, label: "openshift" },
    ]);
  });

  it("orders donut legend entries by descending count", () => {
    const series = buildInventoryDimensionDonutData(intl, {
      aws: 5,
      gcp: 5,
      ibm: 2,
    });

    expect(series.data.map((datum) => datum.x)).toEqual(["aws", "gcp", "ibm"]);
    expect(series.data.map((datum) => datum.y)).toEqual([5, 5, 2]);
  });
});
