import { afterAll, describe, expect, it, vi } from "vitest";

import type { HsctlWorkerResponse } from "./hsctl-worker-protocol";

const posted: HsctlWorkerResponse[] = [];
const fetchMock = vi.fn();
let bridge: (argv: string[]) => unknown = (argv) =>
  Promise.resolve({
    exitCode: 0,
    mutated: false,
    stderr: "",
    stdout: `ran ${argv.join(" ")}\n`,
  });

class FakeGo {
  readonly importObject = {};
  run() {
    (globalThis as { hsctlRun?: unknown }).hsctlRun = (argv: string[]) =>
      bridge(argv);
    return new Promise<void>(() => undefined);
  }
}

vi.stubGlobal("postMessage", (message: HsctlWorkerResponse) => {
  posted.push(message);
});
vi.stubGlobal("importScripts", (url: string) => {
  if (url === "/assets/wasm_exec-abc.js") {
    (globalThis as { Go?: unknown }).Go = FakeGo;
  }
});
vi.stubGlobal("fetch", fetchMock);
const fakeInstance = {} as WebAssembly.Instance;
const fakeModule = {} as WebAssembly.Module;
const instantiateStreaming = vi
  .spyOn(WebAssembly, "instantiateStreaming")
  .mockImplementation(async (source) => {
    const response = await source;
    await response.arrayBuffer();
    return { instance: fakeInstance, module: fakeModule };
  });

afterAll(() => {
  vi.unstubAllGlobals();
  instantiateStreaming.mockRestore();
});

function send(data: unknown) {
  globalThis.dispatchEvent(new MessageEvent("message", { data }));
}

async function waitFor(predicate: () => boolean) {
  for (let attempt = 0; attempt < 100 && !predicate(); attempt += 1) {
    await new Promise((resolve) => {
      setTimeout(resolve, 5);
    });
  }
}

describe("hsctl worker", () => {
  // Verifies: WEB-TERM-10
  it("loads wasm_exec and the module, reports progress, and runs commands", async () => {
    fetchMock.mockResolvedValue(
      new Response(new Uint8Array(512), {
        headers: { "content-length": "512", "content-type": "text/plain" },
      }),
    );
    await import("./hsctl-worker");

    send({
      type: "load",
      wasmExecUrl: "/assets/wasm_exec-abc.js",
      wasmUrl: "/assets/hsctl-abc.wasm",
    });
    await waitFor(() => posted.some((message) => message.type === "loaded"));

    expect(fetchMock).toHaveBeenCalledWith("/assets/hsctl-abc.wasm", {
      credentials: "same-origin",
    });
    expect(posted[0]).toEqual({
      loadedBytes: 0,
      totalBytes: 512,
      type: "progress",
    });
    expect(posted).toContainEqual({
      loadedBytes: 512,
      totalBytes: 512,
      type: "progress",
    });
    const streamed = await instantiateStreaming.mock.calls[0]?.[0];
    expect(streamed?.headers.get("content-type")).toBe("application/wasm");

    send({ argv: ["list", "gateways"], id: 1, type: "run" });
    await waitFor(() => posted.some((message) => message.type === "result"));
    expect(posted.at(-1)).toEqual({
      exitCode: 0,
      id: 1,
      mutated: false,
      stderr: "",
      stdout: "ran list gateways\n",
      type: "result",
    });
  });

  // Verifies: WEB-TERM-10
  it("reports invalid or failing bridge calls as run failures", async () => {
    bridge = () => Promise.resolve({ stdout: 1 });
    send({ argv: ["version"], id: 2, type: "run" });
    await waitFor(() =>
      posted.some((message) => message.type === "run-failed"),
    );
    expect(posted.at(-1)).toEqual({ id: 2, type: "run-failed" });

    bridge = () => Promise.reject(new Error("go exited"));
    send({ argv: ["version"], id: 3, type: "run" });
    await waitFor(() =>
      posted.some(
        (message) => message.type === "run-failed" && message.id === 3,
      ),
    );
    expect(posted.at(-1)).toEqual({ id: 3, type: "run-failed" });
  });
});
