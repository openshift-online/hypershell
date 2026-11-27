import { describe, expect, it, vi } from "vitest";

import { HsctlRuntimeError } from "../../application/terminal/terminal-operations";
import type {
  HsctlWorkerRequest,
  HsctlWorkerResponse,
} from "./hsctl-worker-protocol";
import {
  createWorkerHsctlRuntime,
  type HsctlWorkerHandle,
} from "./worker-hsctl-runtime";

const assets = {
  wasmExecUrl: "/assets/wasm_exec-abc.js",
  wasmUrl: "/assets/hsctl-abc.wasm",
};

class FakeWorker implements HsctlWorkerHandle {
  readonly messages: HsctlWorkerRequest[] = [];
  terminated = false;
  private readonly listeners = new Map<string, ((event: Event) => void)[]>();

  addEventListener(
    type: "error" | "message",
    listener: (event: Event) => void,
  ) {
    this.listeners.set(type, [...(this.listeners.get(type) ?? []), listener]);
  }

  postMessage(message: HsctlWorkerRequest) {
    this.messages.push(message);
  }

  terminate() {
    this.terminated = true;
  }

  emit(message: HsctlWorkerResponse | { type: "unknown" }) {
    for (const listener of this.listeners.get("message") ?? []) {
      listener(new MessageEvent("message", { data: message }));
    }
  }

  fail() {
    for (const listener of this.listeners.get("error") ?? []) {
      listener(new Event("error"));
    }
  }
}

function setup() {
  const workers: FakeWorker[] = [];
  const runtime = createWorkerHsctlRuntime({
    assets,
    createWorker: () => {
      const worker = new FakeWorker();
      workers.push(worker);
      return worker;
    },
  });
  return { runtime, workers };
}

describe("worker hsctl runtime", () => {
  // Verifies: WEB-TERM-09
  it("creates no worker until the runtime is loaded", async () => {
    const { runtime, workers } = setup();
    expect(workers).toHaveLength(0);

    const progress = vi.fn();
    const loading = runtime.load(progress);
    expect(workers).toHaveLength(1);
    expect(workers[0]?.messages[0]).toEqual({ type: "load", ...assets });
    workers[0]?.emit({ loadedBytes: 10, totalBytes: 100, type: "progress" });
    workers[0]?.emit({ type: "unknown" });
    workers[0]?.emit({ type: "loaded" });

    await expect(loading).resolves.toBeUndefined();
    expect(progress).toHaveBeenCalledWith({ loadedBytes: 10, totalBytes: 100 });
    // A second load reuses the worker.
    await runtime.load(progress);
    expect(workers).toHaveLength(1);
  });

  // Verifies: WEB-TERM-04.01
  it("sends argv to the worker and resolves the bridge result", async () => {
    const { runtime, workers } = setup();
    const loading = runtime.load(() => undefined);
    workers[0]?.emit({ type: "loaded" });
    await loading;

    const result = runtime.run(["list", "gateways"]);
    await Promise.resolve();
    expect(workers[0]?.messages[1]).toEqual({
      argv: ["list", "gateways"],
      id: 1,
      type: "run",
    });
    workers[0]?.emit({
      exitCode: 0,
      id: 99,
      mutated: false,
      stderr: "",
      stdout: "stale",
      type: "result",
    });
    workers[0]?.emit({
      exitCode: 0,
      id: 1,
      mutated: false,
      stderr: "",
      stdout: "NAME\n",
      type: "result",
    });
    await expect(result).resolves.toEqual({
      exitCode: 0,
      mutated: false,
      stderr: "",
      stdout: "NAME\n",
    });
  });

  // Verifies: WEB-TERM-05.01
  it("cancels by terminating the worker and starts a new one next time", async () => {
    const { runtime, workers } = setup();
    const loading = runtime.load(() => undefined);
    workers[0]?.emit({ type: "loaded" });
    await loading;

    const running = runtime.run(["list", "gateways"]);
    runtime.cancel();
    await expect(running).rejects.toMatchObject({ kind: "cancelled" });
    expect(workers[0]?.terminated).toBe(true);

    const next = runtime.run(["version"]);
    expect(workers).toHaveLength(2);
    workers[1]?.emit({ type: "loaded" });
    await Promise.resolve();
    await Promise.resolve();
    expect(workers[1]?.messages.at(-1)).toMatchObject({
      argv: ["version"],
      type: "run",
    });
    workers[1]?.emit({ id: 2, type: "run-failed" });
    await expect(next).rejects.toMatchObject({ kind: "crashed" });
    // A failed run discards the worker; the next command gets a fresh one.
    expect(workers[1]?.terminated).toBe(true);
    void runtime.run(["version"]).catch(() => undefined);
    expect(workers).toHaveLength(3);
  });

  // Verifies: WEB-TERM-10
  it("discards the worker when the Go program exits", async () => {
    const { runtime, workers } = setup();
    const loading = runtime.load(() => undefined);
    workers[0]?.emit({ type: "exited" });
    await expect(loading).rejects.toMatchObject({ kind: "load-failed" });

    const reload = runtime.load(() => undefined);
    workers[1]?.emit({ type: "loaded" });
    await reload;
    const running = runtime.run(["version"]);
    await Promise.resolve();
    workers[1]?.emit({ type: "exited" });
    await expect(running).rejects.toMatchObject({ kind: "crashed" });
    expect(workers[1]?.terminated).toBe(true);
  });

  // Verifies: WEB-TERM-09
  it("rejects load failures and allows a retry with a new worker", async () => {
    const { runtime, workers } = setup();
    const first = runtime.load(() => undefined);
    workers[0]?.emit({ stage: "download", type: "load-failed" });
    await expect(first).rejects.toBeInstanceOf(HsctlRuntimeError);
    expect(workers[0]?.terminated).toBe(true);

    const second = runtime.load(() => undefined);
    workers[1]?.fail();
    await expect(second).rejects.toMatchObject({ kind: "load-failed" });
  });

  // Verifies: WEB-TERM-10
  it("reports a worker crash during a command and refuses a second command", async () => {
    const { runtime, workers } = setup();
    const loading = runtime.load(() => undefined);
    workers[0]?.emit({ type: "loaded" });
    await loading;
    const running = runtime.run(["list", "gateways"]);
    await expect(runtime.run(["version"])).rejects.toMatchObject({
      kind: "crashed",
    });
    workers[0]?.fail();
    await expect(running).rejects.toMatchObject({ kind: "crashed" });
  });

  // Verifies: WEB-TERM-09
  it("reports an unavailable runtime when the build has no assets", async () => {
    const runtime = createWorkerHsctlRuntime({
      assets: null,
      createWorker: () => new FakeWorker(),
    });
    await expect(runtime.load(() => undefined)).rejects.toMatchObject({
      kind: "unavailable",
    });
    await expect(runtime.run(["version"])).rejects.toMatchObject({
      kind: "unavailable",
    });
  });
});
