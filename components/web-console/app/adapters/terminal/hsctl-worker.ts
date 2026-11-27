/*
 * Dedicated classic worker that hosts the `hsctl` js/wasm runtime
 * (WEB-TERM-10, CLI-WASM-02). Vite bundles this file into one self-contained
 * script, `assets/hsctl-worker-<hash>.js`, which the BFF serves with the only
 * CSP that allows `'wasm-unsafe-eval'`.
 *
 * The worker loads Go's `wasm_exec.js` with `importScripts`, compiles and
 * starts `hsctl.wasm`, and then answers `run` requests through the single
 * `hsctlRun(argv)` global the module defines. The module itself sends API
 * requests to the same-origin `/api` proxy with the session cookie; this
 * worker never sees or adds credentials. Cancelling a command terminates the
 * whole worker, so no state survives into the next command.
 */

import {
  toBridgeResult,
  type HsctlWorkerLoadStage,
  type HsctlWorkerRequest,
  type HsctlWorkerResponse,
} from "./hsctl-worker-protocol";

interface GoProgram {
  readonly importObject: WebAssembly.Imports;
  run(instance: WebAssembly.Instance): Promise<void>;
}

interface HsctlWorkerScope {
  Go?: new () => GoProgram;
  addEventListener(
    type: "message",
    listener: (event: MessageEvent<unknown>) => void,
  ): void;
  hsctlRun?: (argv: string[]) => unknown;
  importScripts(...urls: string[]): void;
  postMessage(message: HsctlWorkerResponse): void;
}

const scope = globalThis as unknown as HsctlWorkerScope;
const progressStepBytes = 256 * 1024;
const bridgeWaitAttempts = 50;
const bridgeWaitMilliseconds = 20;

class LoadStageError extends Error {
  readonly stage: HsctlWorkerLoadStage;

  constructor(stage: HsctlWorkerLoadStage) {
    super(stage);
    this.stage = stage;
  }
}

function post(message: HsctlWorkerResponse) {
  scope.postMessage(message);
}

// Wraps the download in a counting stream, so progress reports bytes that have
// actually arrived. A compressed response has no trustworthy total size.
async function downloadWasm(url: string): Promise<Response> {
  let response: Response;
  try {
    response = await fetch(url, { credentials: "same-origin" });
  } catch {
    throw new LoadStageError("download");
  }
  if (!response.ok || response.body === null) {
    throw new LoadStageError("download");
  }
  const encoding = response.headers.get("content-encoding");
  const length = Number(response.headers.get("content-length"));
  const totalBytes =
    (encoding === null || encoding === "identity") &&
    Number.isFinite(length) &&
    length > 0
      ? length
      : null;
  let loadedBytes = 0;
  let reportedBytes = 0;
  post({ loadedBytes, totalBytes, type: "progress" });
  const counted = response.body.pipeThrough(
    new TransformStream<Uint8Array, Uint8Array>({
      flush() {
        post({ loadedBytes, totalBytes, type: "progress" });
      },
      transform(chunk, controller) {
        loadedBytes += chunk.byteLength;
        if (loadedBytes - reportedBytes >= progressStepBytes) {
          reportedBytes = loadedBytes;
          post({ loadedBytes, totalBytes, type: "progress" });
        }
        controller.enqueue(chunk);
      },
    }),
  );
  return new Response(counted, {
    headers: { "content-type": "application/wasm" },
  });
}

async function instantiate(
  response: Response,
  imports: WebAssembly.Imports,
): Promise<WebAssembly.Instance> {
  try {
    if (typeof WebAssembly.instantiateStreaming === "function") {
      return (await WebAssembly.instantiateStreaming(response, imports))
        .instance;
    }
    const bytes = await response.arrayBuffer();
    return (await WebAssembly.instantiate(bytes, imports)).instance;
  } catch {
    throw new LoadStageError("instantiate");
  }
}

async function waitForBridge(): Promise<void> {
  for (let attempt = 0; attempt < bridgeWaitAttempts; attempt += 1) {
    if (typeof scope.hsctlRun === "function") {
      return;
    }
    await new Promise((resolve) => {
      setTimeout(resolve, bridgeWaitMilliseconds);
    });
  }
  throw new LoadStageError("bridge");
}

let loading: Promise<void> | null = null;

async function load(wasmExecUrl: string, wasmUrl: string): Promise<void> {
  try {
    scope.importScripts(wasmExecUrl);
  } catch {
    throw new LoadStageError("wasm-exec");
  }
  const GoConstructor = scope.Go;
  if (GoConstructor === undefined) {
    throw new LoadStageError("wasm-exec");
  }
  const go = new GoConstructor();
  const instance = await instantiate(
    await downloadWasm(wasmUrl),
    go.importObject,
  );
  // The Go program keeps running until the worker is terminated. If it exits
  // anyway, the page discards this worker and starts a fresh one.
  go.run(instance).then(
    () => {
      post({ type: "exited" });
    },
    () => {
      post({ type: "exited" });
    },
  );
  await waitForBridge();
}

async function run(id: number, argv: readonly string[]): Promise<void> {
  const bridge = scope.hsctlRun;
  if (typeof bridge !== "function") {
    post({ id, type: "run-failed" });
    return;
  }
  try {
    const result = toBridgeResult(await bridge([...argv]));
    post(
      result === null
        ? { id, type: "run-failed" }
        : { ...result, id, type: "result" },
    );
  } catch {
    post({ id, type: "run-failed" });
  }
}

scope.addEventListener("message", (event) => {
  const request = event.data as HsctlWorkerRequest;
  if (request.type === "load") {
    loading ??= load(request.wasmExecUrl, request.wasmUrl);
    loading.then(
      () => {
        post({ type: "loaded" });
      },
      (error: unknown) => {
        post({
          stage: error instanceof LoadStageError ? error.stage : "instantiate",
          type: "load-failed",
        });
      },
    );
    return;
  }
  void run(request.id, request.argv);
});
