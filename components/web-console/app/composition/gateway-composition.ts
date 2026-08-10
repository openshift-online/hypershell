import { createGatewayOperations } from "@openshift-online/hypershell-gateway-management-ui";

import { createApiClient } from "../adapters/api/api.client";
import { createGatewayControlPlaneAdapter } from "../adapters/api/gateway-operations";
import { gatewayObservability } from "../adapters/observability/gateway-observability";

const gatewayControlPlane = createGatewayControlPlaneAdapter((correlationId) =>
  createApiClient(correlationId),
);

export const gatewayOperations = createGatewayOperations({
  controlPlane: gatewayControlPlane,
  probes: gatewayObservability.probes,
  runtime: gatewayObservability.runtime,
});
