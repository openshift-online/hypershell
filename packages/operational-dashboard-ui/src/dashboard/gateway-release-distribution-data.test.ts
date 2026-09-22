import { describe, expect, it } from "vitest";

import { buildGatewayReleaseListEntries } from "./gateway-release-distribution-data";

describe("buildGatewayReleaseListEntries", () => {
  it("sorts release distribution entries by descending count", () => {
    expect(
      buildGatewayReleaseListEntries({
        "OpenShell 2.0": 4,
        "OpenShell 2.1": 2,
        unknown: 1,
      }),
    ).toEqual([
      { count: 4, label: "OpenShell 2.0" },
      { count: 2, label: "OpenShell 2.1" },
      { count: 1, label: "unknown" },
    ]);
  });

  it("omits zero-count buckets", () => {
    expect(
      buildGatewayReleaseListEntries({
        "OpenShell 2.0": 0,
        "OpenShell 2.1": 2,
      }),
    ).toEqual([{ count: 2, label: "OpenShell 2.1" }]);
  });

  it("returns an empty list when distribution is absent", () => {
    expect(buildGatewayReleaseListEntries(undefined)).toEqual([]);
  });
});
