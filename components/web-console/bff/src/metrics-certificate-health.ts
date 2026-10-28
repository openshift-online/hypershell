import type { MetricsSource } from "./metrics-source.js";
import {
  queryPrometheusInstantNumber,
  queryPrometheusInstantNumberAllowAbsent,
} from "./prometheus-instant-query.js";

export const validCertificatesPromql =
  "sum(max by (hypershell_cluster_id) (gateway_certificate_valid))";
export const expiredCertificatesPromql =
  "sum(max by (hypershell_cluster_id) (gateway_certificate_expired))";
export const unavailableCertificatesPromql =
  "sum(max by (hypershell_cluster_id) (gateway_certificate_unavailable))";
export const soonestCertificateExpiryPromql =
  "min(max by (hypershell_cluster_id) (gateway_certificate_days_to_expiry_min))";

export interface CertificateHealthMetrics {
  valid_certificates: number;
  total_certificates: number;
  expired_certificates: number;
  status_unavailable: number;
  soonest_days_to_expiry: number | null;
}

export async function queryCertificateHealth(
  source: MetricsSource,
  timeoutMs: number,
): Promise<CertificateHealthMetrics> {
  const [valid, expired, unavailable, soonest] = await Promise.all([
    queryPrometheusInstantNumber(source, validCertificatesPromql, timeoutMs),
    queryPrometheusInstantNumber(source, expiredCertificatesPromql, timeoutMs),
    queryPrometheusInstantNumber(
      source,
      unavailableCertificatesPromql,
      timeoutMs,
    ),
    queryPrometheusInstantNumberAllowAbsent(
      source,
      soonestCertificateExpiryPromql,
      timeoutMs,
    ).catch(() => undefined),
  ]);

  return {
    valid_certificates: Math.round(valid),
    total_certificates: Math.round(valid + expired + unavailable),
    expired_certificates: Math.round(expired),
    status_unavailable: Math.round(unavailable),
    soonest_days_to_expiry: soonest ?? null,
  };
}
