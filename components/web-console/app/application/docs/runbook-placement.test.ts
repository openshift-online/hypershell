import { describe, expect, it } from "vitest";

import {
  chooseRunbookPlacement,
  isGatewayEligibleForRunbook,
  runbookAvailabilitySelectors,
  runbookGatewayStage,
} from "./runbook-placement";

function availability(
  entries: readonly (readonly [readonly string[], boolean])[],
) {
  return {
    placements: entries.map(([selector, available]) => ({
      available,
      selector,
      ...(available ? {} : { reason: "no-eligible-cluster" as const }),
    })),
  };
}

describe("runbook placement", () => {
  // Verifies: WEB-UDOC-06
  it("asks one batch request about Kind, IBM Cloud public, and AWS public", () => {
    expect(runbookAvailabilitySelectors("github-jira")).toEqual([
      ["provider/kind"],
      ["network/public", "provider/ibm"],
      ["network/public", "provider/aws"],
    ]);
    expect(runbookAvailabilitySelectors("gitlab-jira")).toEqual([
      ["network/vpn", "provider/aws"],
    ]);
  });

  // Verifies: WEB-UDOC-06
  it("takes the first available public candidate in priority order", () => {
    const decision = chooseRunbookPlacement(
      "github-jira",
      availability([
        [["provider/kind"], false],
        [["provider/ibm", "network/public"], true],
        [["network/public", "provider/aws"], true],
      ]),
    );
    expect(decision).toMatchObject({
      candidate: {
        id: "ibm-public",
        placement: {
          excludedSelector: ["runtime/kata"],
          selector: ["network/public", "provider/ibm"],
        },
      },
      kind: "available",
    });

    expect(
      chooseRunbookPlacement(
        "github-jira",
        availability([
          [["provider/kind"], true],
          [["network/public", "provider/ibm"], true],
        ]),
      ),
    ).toMatchObject({ candidate: { id: "kind" }, kind: "available" });

    expect(
      chooseRunbookPlacement(
        "github-jira",
        availability([
          [["provider/kind"], false],
          [["network/public", "provider/ibm"], false],
          [["network/public", "provider/aws"], true],
        ]),
      ),
    ).toMatchObject({
      candidate: { id: "aws-public" },
      kind: "available",
    });
  });

  // Verifies: WEB-UDOC-06.01
  it("reports no public placement when no candidate is available", () => {
    expect(
      chooseRunbookPlacement(
        "github-jira",
        availability([
          [["provider/kind"], false],
          [["network/public", "provider/ibm"], false],
          [["network/public", "provider/aws"], false],
        ]),
      ),
    ).toEqual({ kind: "unavailable", reason: "no-eligible-cluster" });
  });

  // Verifies: WEB-UDOC-07, WEB-UDOC-07.01
  it("uses only the Red Hat VPN placement for the GitLab runbook", () => {
    expect(
      chooseRunbookPlacement(
        "gitlab-jira",
        availability([
          [["network/vpn", "provider/aws"], false],
          [["network/public", "provider/aws"], true],
        ]),
      ),
    ).toEqual({ kind: "unavailable", reason: "no-eligible-cluster" });
    expect(
      chooseRunbookPlacement(
        "gitlab-jira",
        availability([[["provider/aws", "network/vpn"], true]]),
      ),
    ).toMatchObject({
      candidate: { placement: { selector: ["network/vpn", "provider/aws"] } },
      kind: "available",
    });
  });

  // Verifies: WEB-UDOC-07
  it("admits only gateways on a Red Hat VPN cluster to the GitLab runbook", () => {
    const capabilities = new Map([
      ["vpn-cluster", ["network/vpn", "provider/aws"]],
      ["public-cluster", ["network/public", "provider/aws"]],
    ]);
    expect(
      isGatewayEligibleForRunbook(
        "gitlab-jira",
        { clusterId: "vpn-cluster" },
        capabilities,
      ),
    ).toBe(true);
    expect(
      isGatewayEligibleForRunbook(
        "gitlab-jira",
        { clusterId: "public-cluster" },
        capabilities,
      ),
    ).toBe(false);
    expect(
      isGatewayEligibleForRunbook(
        "gitlab-jira",
        { clusterId: "" },
        capabilities,
      ),
    ).toBe(false);
    expect(
      isGatewayEligibleForRunbook(
        "github-jira",
        { clusterId: "public-cluster" },
        new Map(),
      ),
    ).toBe(true);
  });

  // Verifies: WEB-UDOC-06
  it("names the lifecycle stage, including waiting for placement", () => {
    expect(runbookGatewayStage({ clusterId: "", phase: "Pending" })).toBe(
      "waiting-for-placement",
    );
    expect(runbookGatewayStage({ clusterId: "c1", phase: "Pending" })).toBe(
      "pending",
    );
    expect(
      runbookGatewayStage({ clusterId: "c1", phase: "Provisioning" }),
    ).toBe("provisioning");
    expect(runbookGatewayStage({ clusterId: "c1", phase: "Running" })).toBe(
      "running",
    );
    expect(runbookGatewayStage({ clusterId: "c1", phase: "Failed" })).toBe(
      "failed",
    );
    expect(runbookGatewayStage({ clusterId: "c1", phase: "Degraded" })).toBe(
      "degraded",
    );
    expect(runbookGatewayStage({ clusterId: "c1", phase: "Updating" })).toBe(
      "updating",
    );
  });
});
