import type {
  DomainProbe,
  DomainProbePublisher,
} from "@openshift-online/hypershell-domain-probes";

import type { GatewayFailureKind } from "./gateway-types";

export type GatewayAction =
  | "create-service-account"
  | "delete-service-account"
  | "find-placements"
  | "get"
  | "get-placement"
  | "get-placements"
  | "list"
  | "list-service-accounts"
  | "provision"
  | "remove"
  | "rename"
  | "get-service-account"
  | "revoke-service-account";
export type GatewayProbeOutcome =
  "started" | "succeeded" | "failed" | "cancelled" | "denied" | "conflicted";
export type GatewayProbeName =
  | "gateway.workflow.started"
  | "gateway.workflow.completed"
  | "gateway.dependency.attempted"
  | "gateway.dependency.completed";

export type GatewayProbe = DomainProbe<
  GatewayProbeName,
  1,
  {
    readonly action: GatewayAction;
    readonly failureKind: GatewayFailureKind | null;
    readonly outcome: GatewayProbeOutcome;
  }
>;

export type GatewayProbePublisher = DomainProbePublisher<GatewayProbe>;

export const gatewayProbeCatalog = Object.freeze([
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
    name: "gateway.workflow.started",
    owner: "gateway",
    schemaVersion: 1,
    trigger: "A gateway application use case starts",
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
    name: "gateway.workflow.completed",
    owner: "gateway",
    schemaVersion: 1,
    trigger: "A gateway application use case reaches one terminal outcome",
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
    name: "gateway.dependency.attempted",
    owner: "gateway",
    schemaVersion: 1,
    trigger: "A gateway control-plane dependency attempt starts",
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
    name: "gateway.dependency.completed",
    owner: "gateway",
    schemaVersion: 1,
    trigger: "A gateway control-plane dependency attempt completes",
  },
] as const);
