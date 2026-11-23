import type {
  DomainProbe,
  DomainProbePublisher,
} from "@openshift-online/hypershell-domain-probes";

import type { PlatformPolicyFailureKind } from "./platform-policy-types";

/*
 * Closed, versioned domain facts for the platform policy admin panel
 * (WEB-PADM-10, UIS-OBS-01/02/07). Fields are bounded enums, booleans, and
 * counts only: no Cedar text, user message, policy name, or credential ever
 * enters a probe payload.
 */

export type PlatformPolicyAction =
  | "check-access"
  | "create"
  | "delete"
  | "evaluate"
  | "get"
  | "get-settings"
  | "list"
  | "list-findings"
  | "set-global-mode"
  | "update";

export type PlatformPolicyOutcome =
  "cancelled" | "conflicted" | "denied" | "failed" | "started" | "succeeded";

export type PlatformPolicyWriteAction =
  "create" | "delete" | "set-global-mode" | "update";

type Context = DomainProbe["context"];

export type PlatformPolicyWorkflowProbe = DomainProbe<
  | "platform-policy.workflow.started"
  | "platform-policy.workflow.completed"
  | "platform-policy.dependency.attempted"
  | "platform-policy.dependency.completed",
  1,
  {
    readonly action: PlatformPolicyAction;
    readonly failureKind: PlatformPolicyFailureKind | null;
    readonly outcome: PlatformPolicyOutcome;
  }
>;

/** Policy list, detail, findings, or settings were loaded for display. */
export type PlatformPolicyLoadedProbe = DomainProbe<
  "platform-policy.loaded",
  1,
  {
    readonly action: "get" | "get-settings" | "list" | "list-findings";
    readonly itemCount: number;
  }
>;

/** The editor dry run returned a validation outcome (WEB-PADM-05/09). */
export type PlatformPolicyValidatedProbe = DomainProbe<
  "platform-policy.validated",
  1,
  {
    readonly diagnosticCount: number;
    readonly forbidsDefaults: boolean;
    readonly requestAllowed: boolean;
    readonly valid: boolean;
  }
>;

/** The administrator confirmed a global mode change (WEB-PADM-03). */
export type PlatformPolicyModeChangeConfirmedProbe = DomainProbe<
  "platform-policy.mode-change.confirmed",
  1,
  {
    readonly from: "audit" | "enforce";
    readonly to: "audit" | "enforce";
  }
>;

export type PlatformPolicySavedProbe = DomainProbe<
  "platform-policy.saved",
  1,
  { readonly action: PlatformPolicyWriteAction }
>;

/** A write was refused (`403` denied or `409` conflicted, WEB-PADM-01/10). */
export type PlatformPolicySaveDeniedProbe = DomainProbe<
  "platform-policy.save.denied",
  1,
  {
    readonly action: PlatformPolicyWriteAction;
    readonly failureKind: "conflicted" | "denied";
  }
>;

export type PlatformPolicyProbe =
  | PlatformPolicyLoadedProbe
  | PlatformPolicyModeChangeConfirmedProbe
  | PlatformPolicySaveDeniedProbe
  | PlatformPolicySavedProbe
  | PlatformPolicyValidatedProbe
  | PlatformPolicyWorkflowProbe;

export type PlatformPolicyProbeName = PlatformPolicyProbe["name"];

export type PlatformPolicyProbePublisher =
  DomainProbePublisher<PlatformPolicyProbe>;

export function freezeProbe<TProbe extends PlatformPolicyProbe>(
  name: TProbe["name"],
  context: Context,
  occurredAt: string,
  fields: TProbe["fields"],
): TProbe {
  return Object.freeze({
    context: Object.freeze({ ...context }),
    fields: Object.freeze({ ...fields }),
    name,
    occurredAt,
    schemaVersion: 1,
  }) as TProbe;
}

const consumers = [
  "structured-log",
  "performance",
  "product-health",
  "trace",
] as const;

export const platformPolicyProbeCatalog = Object.freeze([
  ...(
    [
      ["platform-policy.workflow.started", "A policy admin use case starts"],
      [
        "platform-policy.workflow.completed",
        "A policy admin use case reaches one terminal outcome",
      ],
      [
        "platform-policy.dependency.attempted",
        "A policy API dependency attempt starts",
      ],
      [
        "platform-policy.dependency.completed",
        "A policy API dependency attempt completes",
      ],
    ] as const
  ).map(([name, trigger]) => ({
    allowedConsumers: consumers,
    deliveryClass: "best-effort",
    fields: {
      action: "bounded operational enum",
      failureKind: "bounded operational enum; no raw error content",
      outcome: "bounded operational enum",
    },
    name,
    owner: "platform-policy",
    schemaVersion: 1,
    trigger,
  })),
  {
    allowedConsumers: consumers,
    deliveryClass: "best-effort",
    fields: {
      action: "bounded operational enum",
      itemCount: "non-negative count",
    },
    name: "platform-policy.loaded",
    owner: "platform-policy",
    schemaVersion: 1,
    trigger: "Policies, findings, or settings load for display",
  },
  {
    allowedConsumers: consumers,
    deliveryClass: "best-effort",
    fields: {
      diagnosticCount: "non-negative count",
      forbidsDefaults: "boolean",
      requestAllowed: "boolean",
      valid: "boolean",
    },
    name: "platform-policy.validated",
    owner: "platform-policy",
    schemaVersion: 1,
    trigger: "The editor dry run returns a validation outcome",
  },
  {
    allowedConsumers: consumers,
    deliveryClass: "best-effort",
    fields: { from: "mode enum", to: "mode enum" },
    name: "platform-policy.mode-change.confirmed",
    owner: "platform-policy",
    schemaVersion: 1,
    trigger: "The administrator confirms a global mode change",
  },
  {
    allowedConsumers: consumers,
    deliveryClass: "best-effort",
    fields: { action: "bounded write enum" },
    name: "platform-policy.saved",
    owner: "platform-policy",
    schemaVersion: 1,
    trigger: "A policy or settings write succeeds",
  },
  {
    allowedConsumers: consumers,
    deliveryClass: "best-effort",
    fields: {
      action: "bounded write enum",
      failureKind: "denied or conflicted",
    },
    name: "platform-policy.save.denied",
    owner: "platform-policy",
    schemaVersion: 1,
    trigger: "A policy or settings write is refused (403 or 409)",
  },
] as const);
