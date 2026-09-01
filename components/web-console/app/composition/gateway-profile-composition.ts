import { createGatewayProfileOperations } from "@openshift-online/hypershell-gateway-management-ui";

import { createApiClient } from "../adapters/api/api.client";
import { createGatewayProfileControlPlaneAdapter } from "../adapters/api/gateway-profile-operations";
import { createGatewayProfileObservability } from "../adapters/observability/gateway-profile-observability";

const gatewayProfileObservability = createGatewayProfileObservability();

const gatewayProfileControlPlane = createGatewayProfileControlPlaneAdapter(
  (correlationId) => createApiClient(correlationId),
);

export const gatewayProfileOperations = createGatewayProfileOperations({
  controlPlane: gatewayProfileControlPlane,
  probes: gatewayProfileObservability.probes,
  runtime: gatewayProfileObservability.runtime,
});
