import type {
  DomainProbe,
  DomainProbePublisher,
} from "@openshift-online/hypershell-domain-probes";

import type { HsctlCommandPath } from "./hsctl-command-paths";

/*
 * Closed, versioned domain facts for the browser hsctl terminal (WEB-TERM-13,
 * UIS-OBS-01/02/07). Fields are bounded enums, booleans, and durations only:
 * no command line, argument, flag value, identifier, or output ever enters a
 * probe payload (WEB-OBS-01).
 */

type Context = DomainProbe["context"];

export type TerminalRuntimeFailureKind = "load-failed" | "unavailable";

export type TerminalExitStatusClass =
  "cancelled" | "non-zero" | "runtime-error" | "zero";

export type TerminalRefusalReason =
  "incomplete-line" | "not-hsctl" | "shell-operator";

/** The drawer was opened; `signedIn` is false when only sign-in shows. */
export type TerminalDrawerOpenedProbe = DomainProbe<
  "terminal.drawer.opened",
  1,
  { readonly signedIn: boolean }
>;

export type TerminalDrawerClosedProbe = DomainProbe<
  "terminal.drawer.closed",
  1,
  Readonly<Record<string, never>>
>;

export type TerminalRuntimeLoadStartedProbe = DomainProbe<
  "terminal.runtime-load.started",
  1,
  Readonly<Record<string, never>>
>;

export type TerminalRuntimeLoadCompletedProbe = DomainProbe<
  "terminal.runtime-load.completed",
  1,
  {
    readonly durationMs: number;
    readonly failureKind: TerminalRuntimeFailureKind | null;
    readonly outcome: "failed" | "succeeded";
  }
>;

export type TerminalCommandStartedProbe = DomainProbe<
  "terminal.command.started",
  1,
  { readonly commandPath: HsctlCommandPath }
>;

export type TerminalCommandFinishedProbe = DomainProbe<
  "terminal.command.finished",
  1,
  {
    readonly commandPath: HsctlCommandPath;
    readonly durationMs: number;
    readonly exitStatusClass: TerminalExitStatusClass;
    readonly mutated: boolean;
    readonly sessionExpired: boolean;
  }
>;

/** A line was refused before reaching the runtime bridge (WEB-TERM-04). */
export type TerminalCommandRefusedProbe = DomainProbe<
  "terminal.command.refused",
  1,
  { readonly reason: TerminalRefusalReason }
>;

export type TerminalProbe =
  | TerminalCommandFinishedProbe
  | TerminalCommandRefusedProbe
  | TerminalCommandStartedProbe
  | TerminalDrawerClosedProbe
  | TerminalDrawerOpenedProbe
  | TerminalRuntimeLoadCompletedProbe
  | TerminalRuntimeLoadStartedProbe;

export type TerminalProbeName = TerminalProbe["name"];

export type TerminalProbePublisher = DomainProbePublisher<TerminalProbe>;

export function freezeTerminalProbe<TProbe extends TerminalProbe>(
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

const consumers = ["structured-log", "performance", "product-health"] as const;

const commandPathField =
  "bounded hsctl command path enum (knownHsctlCommandPaths) or other; no arguments";

export const terminalProbeCatalog = Object.freeze([
  {
    allowedConsumers: consumers,
    deliveryClass: "best-effort",
    fields: { signedIn: "boolean" },
    name: "terminal.drawer.opened",
    owner: "terminal",
    schemaVersion: 1,
    trigger: "The user opens the hsctl terminal drawer",
  },
  {
    allowedConsumers: consumers,
    deliveryClass: "best-effort",
    fields: {},
    name: "terminal.drawer.closed",
    owner: "terminal",
    schemaVersion: 1,
    trigger: "The user closes the hsctl terminal drawer",
  },
  {
    allowedConsumers: consumers,
    deliveryClass: "best-effort",
    fields: {},
    name: "terminal.runtime-load.started",
    owner: "terminal",
    schemaVersion: 1,
    trigger: "The terminal starts loading the hsctl browser runtime",
  },
  {
    allowedConsumers: consumers,
    deliveryClass: "best-effort",
    fields: {
      durationMs: "non-negative integer duration",
      failureKind: "bounded failure enum; no raw error content",
      outcome: "succeeded or failed",
    },
    name: "terminal.runtime-load.completed",
    owner: "terminal",
    schemaVersion: 1,
    trigger: "The runtime load reaches one terminal outcome",
  },
  {
    allowedConsumers: consumers,
    deliveryClass: "best-effort",
    fields: { commandPath: commandPathField },
    name: "terminal.command.started",
    owner: "terminal",
    schemaVersion: 1,
    trigger: "An hsctl command is passed to the runtime bridge",
  },
  {
    allowedConsumers: consumers,
    deliveryClass: "best-effort",
    fields: {
      commandPath: commandPathField,
      durationMs: "non-negative integer duration",
      exitStatusClass: "zero, non-zero, cancelled, or runtime-error",
      mutated: "boolean",
      sessionExpired: "boolean",
    },
    name: "terminal.command.finished",
    owner: "terminal",
    schemaVersion: 1,
    trigger: "An hsctl command reaches one terminal outcome",
  },
  {
    allowedConsumers: consumers,
    deliveryClass: "best-effort",
    fields: { reason: "bounded refusal enum" },
    name: "terminal.command.refused",
    owner: "terminal",
    schemaVersion: 1,
    trigger: "A line is refused before reaching the runtime bridge",
  },
] as const);
