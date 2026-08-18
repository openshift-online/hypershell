import { createGatewayOperations } from "@openshift-online/hypershell-gateway-management-ui";

import { createApiClient } from "../adapters/api/api.client";
import { createGatewayControlPlaneAdapter } from "../adapters/api/gateway-operations";
import { createGatewayObservability } from "../adapters/observability/gateway-observability";
import { createGatewayTracing } from "../adapters/observability/gateway-trace-sink";

// Same-origin OTLP/HTTP traces path the BFF exposes and forwards to the
// collector. Keeping it same-origin means the browser never sees a collector
// address and no cross-origin telemetry endpoint is exposed.
const browserTracesEndpoint = "/telemetry/v1/traces";

const tracing = createGatewayTracing({
  serviceName: "hypershell-web-console",
  tracesEndpoint: browserTracesEndpoint,
});

const gatewayObservability = createGatewayObservability({
  additionalSinks: [tracing.sink],
});

const gatewayControlPlane = createGatewayControlPlaneAdapter((correlationId) =>
  createApiClient(correlationId, undefined, () =>
    tracing.traceParentFor(correlationId),
  ),
);

export const gatewayOperations = createGatewayOperations({
  controlPlane: gatewayControlPlane,
  probes: gatewayObservability.probes,
  runtime: gatewayObservability.runtime,
});
