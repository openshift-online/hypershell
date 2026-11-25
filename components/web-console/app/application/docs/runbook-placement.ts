import {
  kataRuntimeSelector,
  kindPlacementSelector,
  type GatewayPlacementAvailability,
  type GatewayPlacementIntent,
  type GatewayRecord,
} from "@openshift-online/hypershell-gateway-management-ui";

/** The two docs runbooks (WEB-UDOC-06, WEB-UDOC-07). */
export type RunbookId = "github-jira" | "gitlab-jira";

export type RunbookPlacementId =
  "aws-public" | "aws-vpn" | "ibm-public" | "kind";

export interface RunbookPlacementCandidate {
  readonly id: RunbookPlacementId;
  /** Placement intent sent with the create request (WEB-PLC-01). */
  readonly placement: GatewayPlacementIntent;
  /**
   * Selector asked about in the batch availability request (WEB-PLC-04). The
   * request cannot carry an exclusion, so the IBM Cloud public candidate asks
   * about its plain selector, as the gateway create page does.
   */
  readonly availabilitySelector: readonly string[];
}

const kind: RunbookPlacementCandidate = {
  availabilitySelector: [kindPlacementSelector],
  id: "kind",
  placement: { selector: [kindPlacementSelector] },
};
const ibmPublic: RunbookPlacementCandidate = {
  availabilitySelector: ["network/public", "provider/ibm"],
  id: "ibm-public",
  placement: {
    excludedSelector: [kataRuntimeSelector],
    selector: ["network/public", "provider/ibm"],
  },
};
const awsPublic: RunbookPlacementCandidate = {
  availabilitySelector: ["network/public", "provider/aws"],
  id: "aws-public",
  placement: { selector: ["network/public", "provider/aws"] },
};
const awsVpn: RunbookPlacementCandidate = {
  availabilitySelector: ["network/vpn", "provider/aws"],
  id: "aws-vpn",
  placement: { selector: ["network/vpn", "provider/aws"] },
};

/**
 * Placement candidates in priority order. The GitHub and Jira runbook takes the
 * first available of Kind (local development, as in Quick create), IBM Cloud
 * public without Kata, and AWS public; the GitLab and Jira runbook needs a Red
 * Hat VPN placement.
 */
export const runbookPlacementCandidates: Readonly<
  Record<RunbookId, readonly RunbookPlacementCandidate[]>
> = {
  "github-jira": [kind, ibmPublic, awsPublic],
  "gitlab-jira": [awsVpn],
};

/** Selectors for the one batch availability request of a runbook. */
export function runbookAvailabilitySelectors(
  runbook: RunbookId,
): readonly (readonly string[])[] {
  return runbookPlacementCandidates[runbook].map(
    (candidate) => candidate.availabilitySelector,
  );
}

export type RunbookPlacementDecision =
  | {
      readonly kind: "available";
      readonly candidate: RunbookPlacementCandidate;
    }
  | { readonly kind: "unavailable"; readonly reason: "no-eligible-cluster" };

function sameSelector(
  left: readonly string[],
  right: readonly string[],
): boolean {
  return (
    left.length === right.length && left.every((label) => right.includes(label))
  );
}

/** Chooses the first available candidate from one availability response. */
export function chooseRunbookPlacement(
  runbook: RunbookId,
  availability: GatewayPlacementAvailability,
): RunbookPlacementDecision {
  for (const candidate of runbookPlacementCandidates[runbook]) {
    const item = availability.placements.find((placement) =>
      sameSelector(placement.selector, candidate.availabilitySelector),
    );
    if (item?.available === true) {
      return { candidate, kind: "available" };
    }
  }
  return { kind: "unavailable", reason: "no-eligible-cluster" };
}

/**
 * Whether an existing gateway suits a runbook. Any accessible gateway suits the
 * GitHub and Jira runbook. The GitLab and Jira runbook needs a gateway on the
 * Red Hat VPN (WEB-UDOC-07). The gateway API does not return the placement
 * intent, so the runbook reads the `network/vpn` capability label of the
 * managed cluster the gateway is placed on; a gateway still waiting for
 * placement does not qualify.
 */
export function isGatewayEligibleForRunbook(
  runbook: RunbookId,
  gateway: Pick<GatewayRecord, "clusterId">,
  clusterCapabilities: ReadonlyMap<string, readonly string[]>,
): boolean {
  if (runbook === "github-jira") {
    return true;
  }
  return (
    gateway.clusterId !== "" &&
    (clusterCapabilities.get(gateway.clusterId)?.includes("network/vpn") ??
      false)
  );
}

/** Whether a runbook needs cluster capability labels to filter gateways. */
export function runbookNeedsClusterCapabilities(runbook: RunbookId): boolean {
  return runbook === "gitlab-jira";
}

/** Lifecycle stage a runbook cell shows for its gateway (WEB-UDOC-06). */
export type RunbookGatewayStage =
  | "degraded"
  | "failed"
  | "pending"
  | "provisioning"
  | "running"
  | "updating"
  | "waiting-for-placement";

export function runbookGatewayStage(
  gateway: Pick<GatewayRecord, "clusterId" | "phase">,
): RunbookGatewayStage {
  const phase = gateway.phase?.trim().toLowerCase() ?? "";
  if (phase === "running") {
    return "running";
  }
  if (phase === "failed" || phase === "error") {
    return "failed";
  }
  if (phase === "provisioning") {
    return "provisioning";
  }
  if (phase === "degraded") {
    return "degraded";
  }
  if (phase === "" || phase === "pending") {
    // A gateway created before a matching cluster exists stays Pending with
    // no cluster (WEB-PLC-07).
    return gateway.clusterId === "" ? "waiting-for-placement" : "pending";
  }
  return "updating";
}
