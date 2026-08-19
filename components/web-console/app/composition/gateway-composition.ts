import { createGatewayOperations } from "@openshift-online/hypershell-gateway-management-ui";
import type { ProbeDeliveryFailure } from "@openshift-online/hypershell-domain-probes/fan-out";

import { createApiClient } from "../adapters/api/api.client";
import { createGatewayControlPlaneAdapter } from "../adapters/api/gateway-operations";
import { createGatewayObservability } from "../adapters/observability/gateway-observability";
import { createGatewayTracing } from "../adapters/observability/gateway-trace-sink";
import { readBrowserRuntimeConfig } from "./browser-runtime-config";

// Same-origin OTLP/HTTP traces path the BFF exposes and forwards to the
// collector. Keeping it same-origin means the browser never sees a collector
// address and no cross-origin telemetry endpoint is exposed.
const browserTracesEndpoint = "/telemetry/v1/traces";

// The operator's sample ratio reaches the browser through the BFF-injected
// runtime config, so the browser trace root honors the configured rate rather
// than always recording every trace, and agrees with the BFF sampler.
const browserRuntimeConfig = readBrowserRuntimeConfig();

// The trace sink is created before the observability publisher because the
// publisher takes the sink as one of its fan-out targets, yet a failed span
// export must report back into that publisher's delivery health. A late-bound
// reporter breaks the cycle: export failures raised before the publisher exists
// are dropped, which is correct because no span can be exported until the sink
// is wired into the publisher and receiving probes.
let reportDeliveryFailure: (
  failure: Readonly<ProbeDeliveryFailure>,
) => void = () => undefined;

const tracing = createGatewayTracing(
  {
    sampleRatio: browserRuntimeConfig.tracing.sampleRatio,
    serviceName: "hypershell-web-console",
    tracesEndpoint: browserTracesEndpoint,
  },
  {
    reportDeliveryFailure: (failure) => {
      reportDeliveryFailure(failure);
    },
  },
);

const gatewayObservability = createGatewayObservability({
  additionalSinks: [tracing.sink],
});

reportDeliveryFailure = (failure) => {
  gatewayObservability.reportDeliveryFailure(failure);
};

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
