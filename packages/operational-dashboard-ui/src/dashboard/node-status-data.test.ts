import { createIntl, createIntlCache } from "react-intl";
import { describe, expect, it } from "vitest";

import { messages } from "../messages";
import { buildNodeStatusData, NODE_STATUS_ORDER } from "./node-status-data";
import { STATUS_DONUT_COLORS } from "./status-donut-colors";

const intlMessages = Object.fromEntries(
  Object.values(messages).map((message) => [
    message.id,
    message.defaultMessage,
  ]),
);
const intl = createIntl(
  { locale: "en", messages: intlMessages },
  createIntlCache(),
);

describe("buildNodeStatusData", () => {
  it("maps healthy and failed buckets to ready and not-ready labels", () => {
    const result = buildNodeStatusData(intl, {
      failed: 1,
      healthy: 7,
    });

    expect(result.data).toEqual([
      { x: "Ready", y: 7 },
      { x: "Not ready", y: 1 },
    ]);
    expect(result.colorScale).toEqual([
      STATUS_DONUT_COLORS.healthy,
      STATUS_DONUT_COLORS.failed,
    ]);
    expect(result.legendData).toHaveLength(2);
  });

  it("omits zero-count buckets from donut data", () => {
    const result = buildNodeStatusData(intl, {
      failed: 0,
      healthy: 8,
    });

    expect(result.data).toEqual([{ x: "Ready", y: 8 }]);
    expect(result.colorScale).toEqual([STATUS_DONUT_COLORS.healthy]);
    expect(result.legendData).toHaveLength(1);
  });

  it("returns empty series when every bucket is zero", () => {
    const result = buildNodeStatusData(intl, {
      failed: 0,
      healthy: 0,
    });

    expect(result.data).toEqual([]);
    expect(result.colorScale).toEqual([]);
    expect(result.legendData).toEqual([]);
  });

  it("uses the ready bucket before the not-ready bucket", () => {
    expect(NODE_STATUS_ORDER).toEqual(["healthy", "failed"]);
  });
});
