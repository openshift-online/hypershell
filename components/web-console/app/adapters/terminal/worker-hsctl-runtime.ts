import {
  hsctlRuntimeAssets,
  type HsctlRuntimeAssets,
} from "virtual:hsctl-runtime";

import {
  HsctlRuntimeError,
  type HsctlLoadProgress,
  type HsctlRunResult,
  type HsctlRuntime,
} from "../../application/terminal/terminal-operations";
import {
  isHsctlWorkerResponse,
  type HsctlWorkerRequest,
  type HsctlWorkerResponse,
} from "./hsctl-worker-protocol";

/** The subset of `Worker` this adapter uses, for tests and alternatives. */
export interface HsctlWorkerHandle {
  addEventListener(
    type: "error" | "message",
    listener: (event: Event) => void,
  ): void;
  postMessage(message: HsctlWorkerRequest): void;
  terminate(): void;
}

export interface WorkerHsctlRuntimeOptions {
  assets?: HsctlRuntimeAssets | null;
  createWorker?: () => HsctlWorkerHandle;
}

// Vite bundles this classic worker as `assets/hsctl-worker-<hash>.js`. The
// worker script is requested only when this function runs, which happens when
// a signed-in user first opens the terminal (WEB-TERM-09).
function createHsctlWorker(): HsctlWorkerHandle {
  return new Worker(new URL("./hsctl-worker.ts", import.meta.url));
}

interface PendingRun {
  id: number;
  reject: (error: HsctlRuntimeError) => void;
  resolve: (result: HsctlRunResult) => void;
}

interface ActiveWorker {
  handle: HsctlWorkerHandle;
  loaded: Promise<void>;
  rejectLoad: (error: HsctlRuntimeError) => void;
}

/**
 * Implements the `HsctlRuntime` port with one dedicated Web Worker at a time
 * (WEB-TERM-10). Cancelling terminates the worker; the next command starts a
 * new one (CLI-WASM-02).
 */
export function createWorkerHsctlRuntime(
  options: WorkerHsctlRuntimeOptions = {},
): HsctlRuntime {
  const assets =
    options.assets === undefined ? hsctlRuntimeAssets : options.assets;
  const createWorker = options.createWorker ?? createHsctlWorker;
  let active: ActiveWorker | null = null;
  let progressListener: (progress: HsctlLoadProgress) => void = () => undefined;
  let pending: PendingRun | null = null;
  let nextRunId = 1;

  function discard(failure: HsctlRuntimeError) {
    const current = active;
    active = null;
    current?.handle.terminate();
    current?.rejectLoad(failure);
    const run = pending;
    pending = null;
    run?.reject(failure);
  }

  function settleRun(id: number, settle: (run: PendingRun) => void): void {
    const run = pending;
    if (run?.id !== id) {
      return;
    }
    pending = null;
    settle(run);
  }

  function handleMessage(
    message: HsctlWorkerResponse,
    onLoaded: () => void,
    isLoaded: () => boolean,
  ): void {
    switch (message.type) {
      case "progress":
        progressListener({
          loadedBytes: message.loadedBytes,
          totalBytes: message.totalBytes,
        });
        return;
      case "loaded":
        onLoaded();
        return;
      case "load-failed":
        discard(new HsctlRuntimeError("load-failed"));
        return;
      case "result":
        settleRun(message.id, (run) => {
          run.resolve({
            exitCode: message.exitCode,
            mutated: message.mutated,
            stderr: message.stderr,
            stdout: message.stdout,
          });
        });
        return;
      case "run-failed":
        settleRun(message.id, (run) => {
          run.reject(new HsctlRuntimeError("crashed"));
        });
        // The module may be broken; the next command gets a fresh worker.
        discard(new HsctlRuntimeError("crashed"));
        return;
      case "exited":
        // The Go program stopped, so the bridge is gone.
        discard(new HsctlRuntimeError(isLoaded() ? "crashed" : "load-failed"));
        return;
    }
  }

  function startWorker(runtimeAssets: HsctlRuntimeAssets): ActiveWorker {
    const handle = createWorker();
    let isLoaded = false;
    let resolveLoad: () => void = () => undefined;
    let rejectLoad: (error: HsctlRuntimeError) => void = () => undefined;
    const loaded = new Promise<void>((resolve, reject) => {
      resolveLoad = resolve;
      rejectLoad = reject;
    });
    const worker: ActiveWorker = { handle, loaded, rejectLoad };
    handle.addEventListener("error", () => {
      if (active === worker) {
        discard(new HsctlRuntimeError(isLoaded ? "crashed" : "load-failed"));
      }
    });
    handle.addEventListener("message", (event) => {
      const message = (event as MessageEvent<unknown>).data;
      if (active !== worker || !isHsctlWorkerResponse(message)) {
        return;
      }
      handleMessage(
        message,
        () => {
          isLoaded = true;
          resolveLoad();
        },
        () => isLoaded,
      );
    });
    handle.postMessage({
      type: "load",
      wasmExecUrl: runtimeAssets.wasmExecUrl,
      wasmUrl: runtimeAssets.wasmUrl,
    });
    return worker;
  }

  function ensureLoaded(): Promise<void> {
    if (assets === null) {
      return Promise.reject(new HsctlRuntimeError("unavailable"));
    }
    active ??= startWorker(assets);
    return active.loaded;
  }

  return {
    cancel() {
      discard(new HsctlRuntimeError("cancelled"));
    },
    load(onProgress) {
      progressListener = onProgress;
      return ensureLoaded();
    },
    run(argv) {
      if (pending !== null) {
        return Promise.reject(new HsctlRuntimeError("crashed"));
      }
      const id = nextRunId;
      nextRunId += 1;
      return new Promise<HsctlRunResult>((resolve, reject) => {
        pending = { id, reject, resolve };
        ensureLoaded().then(
          () => {
            if (pending?.id === id) {
              active?.handle.postMessage({ argv: [...argv], id, type: "run" });
            }
          },
          (error: unknown) => {
            settleRun(id, (run) => {
              run.reject(
                error instanceof HsctlRuntimeError &&
                  error.kind === "unavailable"
                  ? error
                  : new HsctlRuntimeError("crashed"),
              );
            });
          },
        );
      });
    },
  };
}
