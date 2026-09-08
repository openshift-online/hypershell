import { createIntl, createIntlCache } from "react-intl";
import { describe, expect, it } from "vitest";

import { messages } from "../messages";
import {
  buildGatewayStatusData,
  GATEWAY_STATUS_COLORS,
} from "./gateway-status-data";

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

describe("buildGatewayStatusData", () => {
  it("omits zero-count buckets from donut data", () => {
    const result = buildGatewayStatusData(intl, {
      degraded: 1,
      failed: 0,
      healthy: 5,
      provisioning: 2,
    });

    expect(result.data).toEqual([
      { x: "Healthy", y: 5 },
      { x: "Provisioning", y: 2 },
      { x: "Degraded", y: 1 },
    ]);
    expect(result.colorScale).toEqual([
      GATEWAY_STATUS_COLORS.healthy,
      GATEWAY_STATUS_COLORS.provisioning,
      GATEWAY_STATUS_COLORS.degraded,
    ]);
    expect(result.legendData).toHaveLength(3);
  });

  it("returns empty series when every bucket is zero", () => {
    const result = buildGatewayStatusData(intl, {
      degraded: 0,
      failed: 0,
      healthy: 0,
      provisioning: 0,
    });

    expect(result.data).toEqual([]);
    expect(result.colorScale).toEqual([]);
    expect(result.legendData).toEqual([]);
  });
});
