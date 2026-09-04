import { createIntl, createIntlCache } from "react-intl";
import { describe, expect, it } from "vitest";

import { messages } from "../messages";
import {
  buildPodCapacityData,
  POD_CAPACITY_PHASE_ORDER,
} from "./pod-capacity-data";
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

describe("buildPodCapacityData", () => {
  it("includes phase segments and unused capacity", () => {
    const result = buildPodCapacityData(intl, 548, 2000, {
      failed: 16,
      pending: 12,
      running: 500,
      succeeded: 20,
      unknown: 0,
    });

    expect(result.data).toEqual([
      { x: "Running", y: 500 },
      { x: "Pending", y: 12 },
      { x: "Failed", y: 16 },
      { x: "Succeeded", y: 20 },
      { x: "Unused", y: 1452 },
    ]);
    expect(result.colorScale).toEqual([
      STATUS_DONUT_COLORS.healthy,
      STATUS_DONUT_COLORS.provisioning,
      STATUS_DONUT_COLORS.failed,
      STATUS_DONUT_COLORS.degraded,
      STATUS_DONUT_COLORS.unused,
    ]);
  });

  it("omits zero-count phase buckets but keeps unused when positive", () => {
    const result = buildPodCapacityData(intl, 8, 10, {
      failed: 0,
      pending: 0,
      running: 8,
      succeeded: 0,
      unknown: 0,
    });

    expect(result.data).toEqual([
      { x: "Running", y: 8 },
      { x: "Unused", y: 2 },
    ]);
  });

  it("orders phases before unused capacity", () => {
    expect(POD_CAPACITY_PHASE_ORDER).toEqual([
      "running",
      "pending",
      "failed",
      "succeeded",
      "unknown",
      "unused",
    ]);
  });
});
