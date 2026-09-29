import type { SDKClient } from "@openshift-online/hypershell-sdk";
import { describe, expect, it, vi } from "vitest";

import {
  bucketReleaseId,
  buildGatewayReleasesMetric,
  resolveReleaseLabel,
} from "./gateway-release-distribution-aggregation";

describe("gateway release distribution aggregation helpers", () => {
  it("buckets omitted and blank release IDs as unknown", () => {
    expect(bucketReleaseId(undefined)).toBe("unknown");
    expect(bucketReleaseId(null)).toBe("unknown");
    expect(bucketReleaseId("  ")).toBe("unknown");
    expect(bucketReleaseId(" rel-1 ")).toBe("rel-1");
  });

  it("resolves release labels with unknown, name, and ID fallback precedence", () => {
    const releaseNameById = new Map([
      ["rel-1", "OpenShell 2.0"],
      ["rel-empty", "  "],
    ]);

    expect(resolveReleaseLabel("unknown", releaseNameById)).toBe("unknown");
    expect(resolveReleaseLabel("rel-1", releaseNameById)).toBe("OpenShell 2.0");
    expect(resolveReleaseLabel("rel-empty", releaseNameById)).toBe("rel-empty");
    expect(resolveReleaseLabel("rel-orphan", releaseNameById)).toBe(
      "rel-orphan",
    );
  });

  it("maps aggregation output into the gateway-releases metric", () => {
    expect(
      buildGatewayReleasesMetric({
        releaseDistribution: new Map([
          ["OpenShell 2.0", 4],
          ["OpenShell 2.1", 2],
        ]),
        total: 6,
      }),
    ).toEqual({
      id: "gateway-releases",
      releaseDistribution: {
        "OpenShell 2.0": 4,
        "OpenShell 2.1": 2,
      },
      value: "6",
    });
  });
});

describe("aggregateGatewayReleaseDistribution", () => {
  it("omits gateway-releases when gateway list aggregation fails", async () => {
    const { aggregateGatewayReleaseDistribution } =
      await import("./gateway-release-distribution-aggregation");
    const client = {
      gateways: {
        list: vi.fn().mockRejectedValue(new Error("gateway list failed")),
      },
      gatewayReleases: {
        list: vi.fn().mockResolvedValue({
          items: [],
          page: 1,
          size: 100,
          total: 0,
        }),
      },
    } as unknown as SDKClient;

    await expect(
      aggregateGatewayReleaseDistribution(client, undefined),
    ).rejects.toThrow("gateway list failed");
  });
});
