import type { SDKClient } from "@openshift-online/hypershell-sdk";
import { describe, expect, it, vi } from "vitest";

import {
  aggregateGatewayReleaseDistribution,
  bucketGatewayVersion,
  buildGatewayReleasesMetric,
} from "./gateway-release-distribution-aggregation";

describe("gateway release distribution aggregation", () => {
  it("buckets omitted and blank gateway versions as unknown", () => {
    expect(bucketGatewayVersion(undefined)).toBe("unknown");
    expect(bucketGatewayVersion(null)).toBe("unknown");
    expect(bucketGatewayVersion("  ")).toBe("unknown");
    expect(bucketGatewayVersion(" v1.0.0 ")).toBe("v1.0.0");
  });

  it("aggregates gateway versions across list pages", async () => {
    const firstPageItems = Array.from({ length: 100 }, (_value, index) => ({
      gateway_version: index < 99 ? "v1.0.0" : "v1.1.0",
    }));
    const gatewayListMock = vi
      .fn()
      .mockResolvedValueOnce({
        items: firstPageItems,
        kind: "GatewayList",
        page: 1,
        size: 100,
        total: 101,
      })
      .mockResolvedValueOnce({
        items: [{ gateway_version: "" }],
        kind: "GatewayList",
        page: 2,
        size: 1,
        total: 101,
      });
    const client = {
      gateways: { list: gatewayListMock },
    } as unknown as SDKClient;

    const aggregate = await aggregateGatewayReleaseDistribution(
      client,
      undefined,
    );

    expect(aggregate).toEqual({
      releaseDistribution: new Map([
        ["v1.0.0", 99],
        ["v1.1.0", 1],
        ["unknown", 1],
      ]),
      total: 101,
    });
    expect(gatewayListMock).toHaveBeenNthCalledWith(
      1,
      { orderBy: "name asc", page: 1, size: 100 },
      { signal: undefined },
    );
    expect(gatewayListMock).toHaveBeenNthCalledWith(
      2,
      { orderBy: "name asc", page: 2, size: 100 },
      { signal: undefined },
    );
  });

  it("rejects an inconsistent gateway list page", async () => {
    const gatewayListMock = vi.fn().mockResolvedValue({
      items: [{ gateway_version: "v1.0.0" }],
      kind: "GatewayList",
      page: 2,
      size: 1,
      total: 1,
    });
    const client = {
      gateways: { list: gatewayListMock },
    } as unknown as SDKClient;

    await expect(
      aggregateGatewayReleaseDistribution(client, undefined),
    ).rejects.toThrow("Gateway list response was inconsistent");
  });

  it("maps the aggregate into the gateway-releases metric", () => {
    expect(
      buildGatewayReleasesMetric({
        releaseDistribution: new Map([
          ["v1.0.0", 2],
          ["unknown", 1],
        ]),
        total: 3,
      }),
    ).toEqual({
      id: "gateway-releases",
      releaseDistribution: {
        unknown: 1,
        "v1.0.0": 2,
      },
      value: "3",
    });
  });
});
