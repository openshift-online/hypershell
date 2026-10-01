// Brand-color semantics (app-repo standards/ui/brand-color.spec.md UI-BRAND-03):
// color is NEVER the sole carrier of state - every status resolves to both a
// semantic token *and* a text label. Unknown/absent always resolves to neutral,
// never inferred success.

export type SemanticTone =
  | "success" // success-green: passed / healthy / synced
  | "warning" // yellow: caution / in progress
  | "danger" // danger-orange: failed / degraded (NOT brand red)
  | "info" // teal: neutral informational
  | "unknown"; // gray: absent / unrecognized / disabled

export interface StatusBadge {
  readonly tone: SemanticTone;
  /** Stable label key resolved to copy by the presentation layer via i18n. */
  readonly labelKey: string;
}

/** Promotion-gate phase → badge. Phases come from the promoter commit statuses. */
export function gatePhaseBadge(phase: string | null | undefined): StatusBadge {
  switch ((phase ?? "").toLowerCase()) {
    case "success":
    case "passed":
      return { tone: "success", labelKey: "passed" };
    case "failure":
    case "failed":
      return { tone: "danger", labelKey: "failed" };
    case "pending":
    case "running":
      return { tone: "warning", labelKey: "pending" };
    default:
      return { tone: "unknown", labelKey: "unknown" };
  }
}

/** Argo CD health string → badge. */
export function healthBadge(health: string | null | undefined): StatusBadge {
  switch ((health ?? "").toLowerCase()) {
    case "healthy":
      return { tone: "success", labelKey: "healthy" };
    case "progressing":
      return { tone: "warning", labelKey: "progressing" };
    case "degraded":
    case "missing":
      return { tone: "danger", labelKey: "degraded" };
    case "suspended":
      return { tone: "info", labelKey: "suspended" };
    default:
      return { tone: "unknown", labelKey: "unknown" };
  }
}

/** Argo CD sync string → badge. */
export function syncBadge(sync: string | null | undefined): StatusBadge {
  switch ((sync ?? "").toLowerCase()) {
    case "synced":
      return { tone: "success", labelKey: "synced" };
    case "outofsync":
      return { tone: "warning", labelKey: "outOfSync" };
    default:
      return { tone: "unknown", labelKey: "unknown" };
  }
}
