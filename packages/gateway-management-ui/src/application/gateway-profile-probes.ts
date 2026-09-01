import type {
  DomainProbe,
  DomainProbePublisher,
} from "@openshift-online/hypershell-domain-probes";

import type { GatewayProfileFailureKind } from "./gateway-profile-types";

export type GatewayProfileAction = "create" | "get" | "list" | "remove";
export type GatewayProfileProbeOutcome =
  "started" | "succeeded" | "failed" | "cancelled" | "denied" | "conflicted";
export type GatewayProfileProbeName =
  | "gatewayProfile.workflow.started"
  | "gatewayProfile.workflow.completed"
  | "gatewayProfile.dependency.attempted"
  | "gatewayProfile.dependency.completed";

export type GatewayProfileProbe = DomainProbe<
  GatewayProfileProbeName,
  1,
  {
    readonly action: GatewayProfileAction;
    readonly failureKind: GatewayProfileFailureKind | null;
    readonly outcome: GatewayProfileProbeOutcome;
  }
>;

export type GatewayProfileProbePublisher =
  DomainProbePublisher<GatewayProfileProbe>;

export const gatewayProfileProbeCatalog = Object.freeze([
  {
    allowedConsumers: [
      "structured-log",
      "performance",
      "product-health",
      "trace",
    ],
    deliveryClass: "best-effort",
    fields: {
      action: "bounded operational enum",
      failureKind: "bounded operational enum; no raw error content",
      outcome: "bounded operational enum",
    },
    name: "gatewayProfile.workflow.started",
    owner: "gatewayProfile",
    schemaVersion: 1,
    trigger: "A gateway-profile application use case starts",
  },
  {
    allowedConsumers: [
      "structured-log",
      "performance",
      "product-health",
      "trace",
    ],
    deliveryClass: "best-effort",
    fields: {
      action: "bounded operational enum",
      failureKind: "bounded operational enum; no raw error content",
      outcome: "bounded operational enum",
    },
    name: "gatewayProfile.workflow.completed",
    owner: "gatewayProfile",
    schemaVersion: 1,
    trigger:
      "A gateway-profile application use case reaches one terminal outcome",
  },
  {
    allowedConsumers: [
      "structured-log",
      "performance",
      "product-health",
      "trace",
    ],
    deliveryClass: "best-effort",
    fields: {
      action: "bounded operational enum",
      failureKind: "bounded operational enum; no raw error content",
      outcome: "bounded operational enum",
    },
    name: "gatewayProfile.dependency.attempted",
    owner: "gatewayProfile",
    schemaVersion: 1,
    trigger: "A gateway-profile control-plane dependency attempt starts",
  },
  {
    allowedConsumers: [
      "structured-log",
      "performance",
      "product-health",
      "trace",
    ],
    deliveryClass: "best-effort",
    fields: {
      action: "bounded operational enum",
      failureKind: "bounded operational enum; no raw error content",
      outcome: "bounded operational enum",
    },
    name: "gatewayProfile.dependency.completed",
    owner: "gatewayProfile",
    schemaVersion: 1,
    trigger: "A gateway-profile control-plane dependency attempt completes",
  },
] as const);
