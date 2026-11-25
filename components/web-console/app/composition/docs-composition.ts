import { createGatewayOperations } from "@openshift-online/hypershell-gateway-management-ui";

import { createApiClient } from "../adapters/api/api.client";
import { createGatewayControlPlaneAdapter } from "../adapters/api/gateway-operations";
import { createBrowserRandomSource } from "../adapters/security/browser-random";
import { createQuickCreateNameGenerator } from "../application/homepage/quick-create";
import { createDocsObservability } from "../adapters/observability/docs-observability";
import { createDocsFacts } from "../application/docs/docs-probes";
import {
  createPolicyTransportFor,
  gatewayObservability,
  gatewayTracing,
} from "./gateway-composition";

export const docsObservability = createDocsObservability();

export const docsFacts = createDocsFacts({
  probes: docsObservability.probes,
  runtime: gatewayObservability.runtime,
});

/**
 * Gateway operations for docs runbook cells. They match the console gateway
 * operations except that an expired session does not navigate away: the 401
 * surfaces as a denied run, and the cell shows its own sign-in link with the
 * current docs path as `return_to` (WEB-UDOC-03).
 */
export const docsGatewayOperations = createGatewayOperations({
  controlPlane: createGatewayControlPlaneAdapter(
    (correlationId) =>
      createApiClient(correlationId, null, () =>
        gatewayTracing.traceParentFor(correlationId),
      ),
    createPolicyTransportFor,
  ),
  probes: gatewayObservability.probes,
  runtime: gatewayObservability.runtime,
});

/** New gateway names for runbook Step 1, as Quick create makes them (WEB-HOME-13). */
export function createDocsGatewayNameGenerator(): () => string {
  return createQuickCreateNameGenerator(createBrowserRandomSource());
}
