import { hsctlCommandPathOf } from "./hsctl-command-paths";
import {
  freezeTerminalProbe,
  type TerminalCommandFinishedProbe,
  type TerminalCommandRefusedProbe,
  type TerminalCommandStartedProbe,
  type TerminalDrawerClosedProbe,
  type TerminalDrawerOpenedProbe,
  type TerminalExitStatusClass,
  type TerminalProbePublisher,
  type TerminalRefusalReason,
  type TerminalRuntimeFailureKind,
  type TerminalRuntimeLoadCompletedProbe,
  type TerminalRuntimeLoadStartedProbe,
} from "./terminal-probes";

/** One `hsctlRun` result from the browser runtime bridge (CLI-WASM-02). */
export interface HsctlRunResult {
  readonly exitCode: number;
  readonly mutated: boolean;
  readonly stderr: string;
  readonly stdout: string;
}

/** Truthful download progress; `totalBytes` is null when unknown. */
export interface HsctlLoadProgress {
  readonly loadedBytes: number;
  readonly totalBytes: number | null;
}

export type HsctlRuntimeFailureKind =
  /** The command was cancelled by terminating the runtime. */
  | "cancelled"
  /** The runtime stopped or returned an invalid result. */
  | "crashed"
  /** Downloading or starting the runtime failed. */
  | "load-failed"
  /** This console build has no runtime assets. */
  | "unavailable";

export class HsctlRuntimeError extends Error {
  readonly kind: HsctlRuntimeFailureKind;

  constructor(kind: HsctlRuntimeFailureKind) {
    super(`hsctl runtime ${kind}`);
    this.name = "HsctlRuntimeError";
    this.kind = kind;
  }
}

/**
 * Driven port for the isolated `hsctl` runtime (WEB-TERM-09/10). `run` loads
 * the runtime again when an earlier `cancel` discarded it.
 */
export interface HsctlRuntime {
  cancel(): void;
  load(onProgress: (progress: HsctlLoadProgress) => void): Promise<void>;
  run(argv: readonly string[]): Promise<HsctlRunResult>;
}

/** Driven port: is the console session still usable (WEB-TERM-10)? */
export interface TerminalSessionCheck {
  isSessionExpired(): Promise<boolean>;
}

/** Nondeterministic context for terminal invocations. */
export interface TerminalRuntimeContext {
  createCorrelationId(): string;
  createTraceId(): string;
  /** Monotonic milliseconds, for durations only. */
  elapsedMilliseconds(): number;
  now(): string;
}

export type TerminalLoadOutcome =
  | {
      readonly outcome: "failed";
      readonly failureKind: TerminalRuntimeFailureKind;
    }
  | { readonly outcome: "succeeded" };

export type TerminalCommandOutcome =
  | { readonly kind: "cancelled" }
  | {
      readonly kind: "finished";
      readonly result: HsctlRunResult;
      readonly sessionExpired: boolean;
    }
  | { readonly kind: "runtime-error" };

/** Driving port used by the terminal presentation. */
export interface TerminalOperations {
  cancelCommand(): void;
  drawerClosed(): void;
  drawerOpened(signedIn: boolean): void;
  loadRuntime(
    onProgress: (progress: HsctlLoadProgress) => void,
  ): Promise<TerminalLoadOutcome>;
  refuseCommand(reason: TerminalRefusalReason): void;
  runCommand(argv: readonly string[]): Promise<TerminalCommandOutcome>;
}

export interface TerminalOperationDependencies {
  probes: TerminalProbePublisher;
  runtime: HsctlRuntime;
  runtimeContext: TerminalRuntimeContext;
  session: TerminalSessionCheck;
}

function runtimeFailureKind(error: unknown): HsctlRuntimeFailureKind {
  return error instanceof HsctlRuntimeError ? error.kind : "crashed";
}

/**
 * Creates the terminal's application workflows. Each runtime load and command
 * publishes one started and exactly one terminal fact (UIS-OBS-01), and only a
 * bounded command path ever leaves the command line (WEB-TERM-13).
 */
export function createTerminalOperations({
  probes,
  runtime,
  runtimeContext,
  session,
}: TerminalOperationDependencies): TerminalOperations {
  const newContext = () => ({
    correlationId: runtimeContext.createCorrelationId(),
    traceId: runtimeContext.createTraceId(),
  });
  const durationSince = (start: number) =>
    Math.max(0, Math.round(runtimeContext.elapsedMilliseconds() - start));

  return {
    cancelCommand() {
      runtime.cancel();
    },
    drawerClosed() {
      probes.publish(
        freezeTerminalProbe<TerminalDrawerClosedProbe>(
          "terminal.drawer.closed",
          newContext(),
          runtimeContext.now(),
          {},
        ),
      );
    },
    drawerOpened(signedIn) {
      probes.publish(
        freezeTerminalProbe<TerminalDrawerOpenedProbe>(
          "terminal.drawer.opened",
          newContext(),
          runtimeContext.now(),
          { signedIn },
        ),
      );
    },
    async loadRuntime(onProgress) {
      const context = newContext();
      const start = runtimeContext.elapsedMilliseconds();
      probes.publish(
        freezeTerminalProbe<TerminalRuntimeLoadStartedProbe>(
          "terminal.runtime-load.started",
          context,
          runtimeContext.now(),
          {},
        ),
      );
      let outcome: TerminalLoadOutcome;
      try {
        await runtime.load(onProgress);
        outcome = { outcome: "succeeded" };
      } catch (error) {
        outcome = {
          failureKind:
            runtimeFailureKind(error) === "unavailable"
              ? "unavailable"
              : "load-failed",
          outcome: "failed",
        };
      }
      probes.publish(
        freezeTerminalProbe<TerminalRuntimeLoadCompletedProbe>(
          "terminal.runtime-load.completed",
          context,
          runtimeContext.now(),
          {
            durationMs: durationSince(start),
            failureKind:
              outcome.outcome === "failed" ? outcome.failureKind : null,
            outcome: outcome.outcome,
          },
        ),
      );
      return outcome;
    },
    refuseCommand(reason) {
      probes.publish(
        freezeTerminalProbe<TerminalCommandRefusedProbe>(
          "terminal.command.refused",
          newContext(),
          runtimeContext.now(),
          { reason },
        ),
      );
    },
    async runCommand(argv) {
      const context = newContext();
      const commandPath = hsctlCommandPathOf(argv);
      const start = runtimeContext.elapsedMilliseconds();
      probes.publish(
        freezeTerminalProbe<TerminalCommandStartedProbe>(
          "terminal.command.started",
          context,
          runtimeContext.now(),
          { commandPath },
        ),
      );
      let outcome: TerminalCommandOutcome;
      let exitStatusClass: TerminalExitStatusClass;
      try {
        const result = await runtime.run(argv);
        const sessionExpired =
          result.exitCode !== 0 &&
          (await session.isSessionExpired().catch(() => false));
        outcome = { kind: "finished", result, sessionExpired };
        exitStatusClass = result.exitCode === 0 ? "zero" : "non-zero";
      } catch (error) {
        const cancelled = runtimeFailureKind(error) === "cancelled";
        outcome = { kind: cancelled ? "cancelled" : "runtime-error" };
        exitStatusClass = cancelled ? "cancelled" : "runtime-error";
      }
      probes.publish(
        freezeTerminalProbe<TerminalCommandFinishedProbe>(
          "terminal.command.finished",
          context,
          runtimeContext.now(),
          {
            commandPath,
            durationMs: durationSince(start),
            exitStatusClass,
            mutated: outcome.kind === "finished" && outcome.result.mutated,
            sessionExpired:
              outcome.kind === "finished" && outcome.sessionExpired,
          },
        ),
      );
      return outcome;
    },
  };
}
