import { createPlatformPolicyOperations } from "../application/platform-policies/platform-policy-operations";
import { createApiClient } from "../adapters/api/api.client";
import { createPlatformPolicyControlPlaneAdapter } from "../adapters/api/platform-policies";
import { createPlatformPolicyObservability } from "../adapters/observability/platform-policy-observability";
import {
  createPolicyTransportFor,
  gatewayObservability,
  gatewayTracing,
} from "./gateway-composition";

export const platformPolicyObservability = createPlatformPolicyObservability();

export const platformPolicyOperations = createPlatformPolicyOperations({
  controlPlane: createPlatformPolicyControlPlaneAdapter(
    createPolicyTransportFor,
    (correlationId) =>
      createApiClient(correlationId, undefined, () =>
        gatewayTracing.traceParentFor(correlationId),
      ),
  ),
  probes: platformPolicyObservability.probes,
  runtime: gatewayObservability.runtime,
});
