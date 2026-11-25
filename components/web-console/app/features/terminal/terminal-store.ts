import {
  analyzeCommandLine,
  type CommandLineAnalysis,
} from "../../application/terminal/hsctl-command-line";
import {
  appendTerminalNotice,
  appendTerminalOutput,
  clearTerminalHistory,
  emptyTerminalHistory,
  splitOutputLines,
  startTerminalEntry,
  terminalHistoryLineLimit,
  type TerminalHistory,
  type TerminalNotice,
  type TerminalOutputStream,
} from "../../application/terminal/terminal-history";
import type {
  HsctlLoadProgress,
  TerminalOperations,
} from "../../application/terminal/terminal-operations";
import type { TerminalRuntimeFailureKind } from "../../application/terminal/terminal-probes";

/*
 * In-memory terminal state for one page session (WEB-TERM-07). The store is a
 * plain object read by React through `useSyncExternalStore`, so runbook entry
 * handles stay valid after any number of renders. Nothing here is written to
 * browser storage, a cache, or a URL.
 */

export type TerminalRuntimeStatus =
  | {
      readonly kind: "failed";
      readonly failureKind: TerminalRuntimeFailureKind;
    }
  | { readonly kind: "idle" }
  | { readonly kind: "loading"; readonly progress: HsctlLoadProgress | null }
  | { readonly kind: "ready" };

export type TerminalSessionStatus = "signed-in" | "signed-out" | "unknown";

export interface TerminalSessionState {
  /** Stable identity (for example the OIDC subject) while signed in. */
  readonly identity?: string;
  readonly status: TerminalSessionStatus;
}

export type TerminalAnnouncement =
  | { readonly kind: "cancelled"; readonly serial: number }
  | {
      readonly exitCode: number;
      readonly kind: "finished";
      readonly serial: number;
    }
  | { readonly kind: "runtime-error"; readonly serial: number };

export interface TerminalSnapshot {
  readonly announcement: TerminalAnnouncement | null;
  /** Typed commands, oldest first, for ArrowUp/ArrowDown recall. */
  readonly commandHistory: readonly string[];
  readonly history: TerminalHistory;
  readonly isOpen: boolean;
  readonly running: { readonly entryId: string } | null;
  readonly runtime: TerminalRuntimeStatus;
  readonly scrollRequest: {
    readonly entryId: string;
    readonly serial: number;
  } | null;
  readonly session: TerminalSessionState;
}

export interface RunbookEntryHandle {
  readonly entryId: string;
  appendOutput(lines: readonly string[], stream?: TerminalOutputStream): void;
  finish(outcome: { exitCode: number }): void;
}

export interface TerminalStoreOptions {
  /** Line limit; tests and stories may lower it. */
  lineLimit?: number;
  /** Called when a command may have changed gateway state (WEB-TERM-07). */
  onGatewayStateMayHaveChanged: () => void;
  operations: TerminalOperations;
}

export interface TerminalStore {
  clear: () => void;
  close: () => void;
  getSnapshot: () => TerminalSnapshot;
  /** Ctrl+C with no running command prints the line and `^C`. */
  interrupt: (line: string) => void;
  /** Ctrl+C with a running command cancels it (CLI-WASM-02). */
  cancelRunning: () => void;
  open: (options?: { entryId?: string }) => void;
  retryLoad: () => void;
  setSession: (session: TerminalSessionState) => void;
  startRunbookEntry: (command: string) => RunbookEntryHandle;
  /** Runs one input line; returns false when a command is already running. */
  submit: (line: string) => boolean;
  subscribe: (listener: () => void) => () => void;
}

const refusalNotices: Partial<
  Record<CommandLineAnalysis["kind"], TerminalNotice>
> = {
  incomplete: { kind: "incomplete-line" },
  "shell-operator": { kind: "shell-operators" },
};

function identityKey(session: TerminalSessionState): string | null {
  if (session.status === "unknown") {
    return null;
  }
  return session.status === "signed-in"
    ? `signed-in:${session.identity ?? ""}`
    : "signed-out";
}

export function createTerminalStore({
  lineLimit = terminalHistoryLineLimit,
  onGatewayStateMayHaveChanged,
  operations,
}: TerminalStoreOptions): TerminalStore {
  const listeners = new Set<() => void>();
  let serial = 0;
  let lastIdentity: string | null = null;
  // Bumped on identity change, so results of commands from an earlier
  // identity are not written into the new identity's history.
  let generation = 0;
  let snapshot: TerminalSnapshot = {
    announcement: null,
    commandHistory: [],
    history: emptyTerminalHistory,
    isOpen: false,
    running: null,
    runtime: { kind: "idle" },
    scrollRequest: null,
    session: { status: "unknown" },
  };

  function update(next: Partial<TerminalSnapshot>) {
    snapshot = { ...snapshot, ...next };
    for (const listener of listeners) {
      listener();
    }
  }

  function nextSerial() {
    serial += 1;
    return serial;
  }

  function loadRuntime() {
    if (
      snapshot.session.status !== "signed-in" ||
      snapshot.runtime.kind === "loading" ||
      snapshot.runtime.kind === "ready"
    ) {
      return;
    }
    update({ runtime: { kind: "loading", progress: null } });
    const loadGeneration = generation;
    void operations
      .loadRuntime((progress) => {
        if (snapshot.runtime.kind === "loading") {
          update({ runtime: { kind: "loading", progress } });
        }
      })
      .then((outcome) => {
        if (loadGeneration !== generation) {
          return;
        }
        update({
          runtime:
            outcome.outcome === "succeeded"
              ? { kind: "ready" }
              : { failureKind: outcome.failureKind, kind: "failed" },
        });
      });
  }

  function appendOutput(
    history: TerminalHistory,
    entryId: string,
    text: string,
    stream: TerminalOutputStream,
  ) {
    return appendTerminalOutput(
      history,
      entryId,
      splitOutputLines(text),
      stream,
      lineLimit,
    );
  }

  async function runCommand(entryId: string, argv: readonly string[]) {
    const runGeneration = generation;
    const outcome = await operations.runCommand(argv);
    if (runGeneration !== generation) {
      if (outcome.kind !== "runtime-error") {
        onGatewayStateMayHaveChanged();
      }
      return;
    }
    let history = snapshot.history;
    let announcement: TerminalAnnouncement;
    if (outcome.kind === "finished") {
      history = appendOutput(history, entryId, outcome.result.stdout, "stdout");
      history = appendOutput(history, entryId, outcome.result.stderr, "stderr");
      if (outcome.result.exitCode !== 0) {
        history = appendTerminalNotice(
          history,
          entryId,
          { exitCode: outcome.result.exitCode, kind: "exit-code" },
          lineLimit,
        );
      }
      if (outcome.sessionExpired) {
        history = appendTerminalNotice(
          history,
          entryId,
          { kind: "session-expired" },
          lineLimit,
        );
      }
      announcement = {
        exitCode: outcome.result.exitCode,
        kind: "finished",
        serial: nextSerial(),
      };
    } else {
      history = appendTerminalNotice(
        history,
        entryId,
        { kind: outcome.kind === "cancelled" ? "cancelled" : "runtime-error" },
        lineLimit,
      );
      announcement = { kind: outcome.kind, serial: nextSerial() };
    }
    // Cancelling or a runtime failure discards the worker (CLI-WASM-02). The
    // runtime loads again through the normal progress and Retry UI
    // (WEB-TERM-09) before the next command can run.
    const runtimeDiscarded = outcome.kind !== "finished";
    update({
      announcement,
      history,
      running: null,
      ...(runtimeDiscarded ? { runtime: { kind: "idle" } as const } : {}),
    });
    if (runtimeDiscarded && snapshot.isOpen) {
      loadRuntime();
    }
    // A cancelled command may already have sent a state-changing request.
    if (
      outcome.kind === "cancelled" ||
      (outcome.kind === "finished" && outcome.result.mutated)
    ) {
      onGatewayStateMayHaveChanged();
    }
  }

  const store: TerminalStore = {
    cancelRunning() {
      if (snapshot.running !== null) {
        operations.cancelCommand();
      }
    },
    clear() {
      update({ history: clearTerminalHistory(snapshot.history) });
    },
    close() {
      if (!snapshot.isOpen) {
        return;
      }
      update({ isOpen: false, scrollRequest: null });
      operations.drawerClosed();
    },
    getSnapshot() {
      return snapshot;
    },
    interrupt(line) {
      const started = startTerminalEntry(
        snapshot.history,
        "typed",
        line,
        lineLimit,
      );
      update({
        history: appendTerminalNotice(
          started.history,
          started.entryId,
          { kind: "interrupted" },
          lineLimit,
        ),
      });
    },
    open(options) {
      // Only an explicit entry scrolls; a plain open follows new output.
      const scrollRequest =
        options?.entryId === undefined
          ? null
          : { entryId: options.entryId, serial: nextSerial() };
      if (snapshot.isOpen) {
        update({ scrollRequest });
        return;
      }
      update({ isOpen: true, scrollRequest });
      operations.drawerOpened(snapshot.session.status === "signed-in");
      loadRuntime();
    },
    retryLoad() {
      if (snapshot.runtime.kind === "failed") {
        update({ runtime: { kind: "idle" } });
        loadRuntime();
      }
    },
    setSession(session) {
      const key = identityKey(session);
      const changed =
        key !== null && lastIdentity !== null && key !== lastIdentity;
      if (key !== null) {
        lastIdentity = key;
      }
      if (changed) {
        // Sign-out or a different identity: drop the in-memory history and
        // stop any command that belongs to the earlier identity.
        generation += 1;
        const wasRunning = snapshot.running !== null;
        if (wasRunning) {
          // Cancelling discards the worker, so the runtime must load again
          // through the normal progress and Retry flow.
          operations.cancelCommand();
        }
        update({
          announcement: null,
          commandHistory: [],
          history: clearTerminalHistory(snapshot.history),
          running: null,
          runtime:
            wasRunning || snapshot.runtime.kind === "loading"
              ? { kind: "idle" }
              : snapshot.runtime,
          scrollRequest: null,
          session,
        });
      } else if (
        session.status !== snapshot.session.status ||
        session.identity !== snapshot.session.identity
      ) {
        update({ session });
      }
      if (snapshot.isOpen) {
        loadRuntime();
      }
    },
    startRunbookEntry(command) {
      const started = startTerminalEntry(
        snapshot.history,
        "runbook",
        command,
        lineLimit,
      );
      const entryId = started.entryId;
      const entryGeneration = generation;
      update({ history: started.history });
      let finished = false;
      return {
        appendOutput(lines, stream = "stdout") {
          if (finished || entryGeneration !== generation) {
            return;
          }
          update({
            history: appendTerminalOutput(
              snapshot.history,
              entryId,
              lines,
              stream,
              lineLimit,
            ),
          });
        },
        entryId,
        finish({ exitCode }) {
          if (finished || entryGeneration !== generation) {
            return;
          }
          finished = true;
          if (exitCode !== 0) {
            update({
              history: appendTerminalNotice(
                snapshot.history,
                entryId,
                { exitCode, kind: "exit-code" },
                lineLimit,
              ),
            });
          }
        },
      };
    },
    submit(line) {
      if (snapshot.running !== null) {
        return false;
      }
      const analysis = analyzeCommandLine(line);
      if (analysis.kind === "empty") {
        return true;
      }
      const commandHistory =
        snapshot.commandHistory.at(-1) === line
          ? snapshot.commandHistory
          : [...snapshot.commandHistory, line];
      if (analysis.kind === "clear") {
        update({
          commandHistory,
          history: clearTerminalHistory(snapshot.history),
        });
        return true;
      }
      const started = startTerminalEntry(
        snapshot.history,
        "typed",
        line,
        lineLimit,
      );
      if (analysis.kind !== "run") {
        const notice: TerminalNotice =
          analysis.kind === "not-hsctl"
            ? { kind: "not-hsctl", program: analysis.program }
            : (refusalNotices[analysis.kind] ?? { kind: "shell-operators" });
        operations.refuseCommand(
          analysis.kind === "not-hsctl"
            ? "not-hsctl"
            : analysis.kind === "incomplete"
              ? "incomplete-line"
              : "shell-operator",
        );
        update({
          commandHistory,
          history: appendTerminalNotice(
            started.history,
            started.entryId,
            notice,
            lineLimit,
          ),
        });
        return true;
      }
      // Clearing the polite status first makes an identical completion
      // message for the next command a new announcement (WEB-TERM-12).
      update({
        announcement: null,
        commandHistory,
        history: started.history,
        running: { entryId: started.entryId },
      });
      void runCommand(started.entryId, analysis.argv);
      return true;
    },
    subscribe(listener) {
      listeners.add(listener);
      return () => {
        listeners.delete(listener);
      };
    },
  };
  return store;
}
