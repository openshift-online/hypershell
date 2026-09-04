// TitleCase canonical gateway phases. Mirrors components/api-server/pkg/gatewayhealth
// and packages/gateway-management-ui/src/gateways/gateway-data.ts.
export const gatewayCanonicalPhaseStrings = [
  "Pending",
  "Provisioning",
  "Running",
  "Degraded",
  "Failed",
] as const;

export type GatewayCanonicalPhase =
  (typeof gatewayCanonicalPhaseStrings)[number];

export function emptyGatewayPhaseCounts(): Record<
  GatewayCanonicalPhase,
  number
> {
  return {
    Pending: 0,
    Provisioning: 0,
    Running: 0,
    Degraded: 0,
    Failed: 0,
  };
}
