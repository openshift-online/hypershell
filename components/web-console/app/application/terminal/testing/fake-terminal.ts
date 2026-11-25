import type { TerminalProbe } from "../terminal-probes";
import {
  defaultTerminalPreferences,
  type TerminalLayoutPreferences,
  type TerminalPreferencesStore,
} from "../terminal-layout";
import {
  createTerminalOperations,
  HsctlRuntimeError,
  type HsctlLoadProgress,
  type HsctlRunResult,
  type HsctlRuntime,
  type HsctlRuntimeFailureKind,
  type TerminalOperations,
} from "../terminal-operations";

/*
 * Scriptable stand-ins for the hsctl runtime, used by stories and tests so no
 * real worker or WebAssembly module is ever loaded (WEB-TERM-14).
 */

export interface FakeRuntimeScript {
  /** Never settle the load (loading story). */
  loadForever?: boolean;
  /** Fail the load with this kind. */
  loadFailure?: HsctlRuntimeFailureKind;
  /** Progress reported before the load settles. */
  progress?: HsctlLoadProgress;
  /** Result for each argv; defaults to empty success. */
  respond?: (argv: readonly string[]) => HsctlRunResult | "forever";
}

export interface FakeHsctlRuntime extends HsctlRuntime {
  readonly calls: (readonly string[])[];
  loadCount: number;
  cancelCount: number;
}

export function createFakeHsctlRuntime(
  script: FakeRuntimeScript = {},
): FakeHsctlRuntime {
  let rejectRun: ((error: HsctlRuntimeError) => void) | null = null;
  const runtime: FakeHsctlRuntime = {
    calls: [],
    cancel: () => {
      runtime.cancelCount += 1;
      rejectRun?.(new HsctlRuntimeError("cancelled"));
      rejectRun = null;
    },
    cancelCount: 0,
    load: (onProgress) => {
      runtime.loadCount += 1;
      if (script.progress !== undefined) {
        onProgress(script.progress);
      }
      if (script.loadForever === true) {
        return new Promise<void>(() => undefined);
      }
      return script.loadFailure === undefined
        ? Promise.resolve()
        : Promise.reject(new HsctlRuntimeError(script.loadFailure));
    },
    loadCount: 0,
    run: (argv) => {
      runtime.calls.push([...argv]);
      const response = script.respond?.(argv) ?? {
        exitCode: 0,
        mutated: false,
        stderr: "",
        stdout: "",
      };
      if (response === "forever") {
        return new Promise<HsctlRunResult>((_resolve, reject) => {
          rejectRun = reject;
        });
      }
      return Promise.resolve(response);
    },
  };
  return runtime;
}

export interface FakeTerminal {
  operations: TerminalOperations;
  probes: TerminalProbe[];
  runtime: FakeHsctlRuntime;
  sessionExpired: { value: boolean };
}

export function createFakeTerminal(
  script: FakeRuntimeScript = {},
): FakeTerminal {
  const runtime = createFakeHsctlRuntime(script);
  const probes: TerminalProbe[] = [];
  const sessionExpired = { value: false };
  let clock = 0;
  let id = 0;
  const operations = createTerminalOperations({
    probes: {
      publish: (probe) => {
        probes.push(probe);
      },
    },
    runtime,
    runtimeContext: {
      createCorrelationId: () => {
        id += 1;
        return `correlation-${String(id)}`;
      },
      createTraceId: () => "0123456789abcdef0123456789abcdef",
      elapsedMilliseconds: () => {
        clock += 5;
        return clock;
      },
      now: () => "2026-11-24T12:00:00.000Z",
    },
    session: {
      isSessionExpired: () => Promise.resolve(sessionExpired.value),
    },
  });
  return { operations, probes, runtime, sessionExpired };
}

export function createMemoryTerminalPreferences(
  initial: TerminalLayoutPreferences = defaultTerminalPreferences,
): TerminalPreferencesStore & { saved: TerminalLayoutPreferences[] } {
  let current = initial;
  const saved: TerminalLayoutPreferences[] = [];
  return {
    read: () => current,
    saved,
    write: (preferences) => {
      current = preferences;
      saved.push(preferences);
    },
  };
}
