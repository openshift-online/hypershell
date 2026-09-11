import {
  applicationScalarQuery,
  queryPrometheusInstantScalar,
} from "./prometheus-instant-query.js";
import type { MetricsSource } from "./metrics-source.js";

export const gatewayActiveSandboxesPromql =
  "hypershell_gateways_active_sandboxes_total";

export interface GatewaySandboxesCounts {
  active_sandboxes: number;
}

export async function queryGatewaySandboxes(
  prometheusUrl: MetricsSource,
  timeoutMs: number,
  namespace?: string,
): Promise<GatewaySandboxesCounts> {
  const activeSandboxes = await queryPrometheusInstantScalar(
    prometheusUrl,
    applicationScalarQuery(gatewayActiveSandboxesPromql, namespace),
    timeoutMs,
  );

  return { active_sandboxes: activeSandboxes };
}
