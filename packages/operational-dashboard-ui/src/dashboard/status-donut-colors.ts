/**
 * Status bucket colors aligned with PatternFly Alert and Label status semantics.
 */
export const STATUS_DONUT_COLORS = {
  degraded: "#ffcc17",
  failed: "#b1380b",
  healthy: "#63993d",
  provisioning: "#0066cc",
  /** Matches PatternFly ChartDonutUtilization unused segment fill. */
  unused: "#d2d2d2",
} as const;
