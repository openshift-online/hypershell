/*
 * Messages between the page and the dedicated hsctl worker (WEB-TERM-10).
 * The worker holds the `hsctl` WebAssembly module; the page sends it only an
 * asset location pair and argv arrays, and receives text results.
 */

export type HsctlWorkerRequest =
  | {
      readonly type: "load";
      readonly wasmExecUrl: string;
      readonly wasmUrl: string;
    }
  | {
      readonly argv: readonly string[];
      readonly id: number;
      readonly type: "run";
    };

export type HsctlWorkerLoadStage =
  "bridge" | "download" | "instantiate" | "wasm-exec";

export type HsctlWorkerResponse =
  | { readonly type: "loaded" }
  | { readonly stage: HsctlWorkerLoadStage; readonly type: "load-failed" }
  | {
      readonly loadedBytes: number;
      readonly totalBytes: number | null;
      readonly type: "progress";
    }
  | {
      readonly exitCode: number;
      readonly id: number;
      readonly mutated: boolean;
      readonly stderr: string;
      readonly stdout: string;
      readonly type: "result";
    }
  | { readonly id: number; readonly type: "run-failed" }
  /** The Go program exited; the worker can run no more commands. */
  | { readonly type: "exited" };

/** Validates a bridge result before it crosses the worker boundary. */
export function toBridgeResult(value: unknown): {
  exitCode: number;
  mutated: boolean;
  stderr: string;
  stdout: string;
} | null {
  if (typeof value !== "object" || value === null) {
    return null;
  }
  const record = value as Record<string, unknown>;
  if (
    typeof record.stdout !== "string" ||
    typeof record.stderr !== "string" ||
    typeof record.exitCode !== "number" ||
    !Number.isInteger(record.exitCode) ||
    typeof record.mutated !== "boolean"
  ) {
    return null;
  }
  return {
    exitCode: record.exitCode,
    mutated: record.mutated,
    stderr: record.stderr,
    stdout: record.stdout,
  };
}

const responseTypes: ReadonlySet<string> = new Set([
  "exited",
  "load-failed",
  "loaded",
  "progress",
  "result",
  "run-failed",
]);

/** Narrows an untrusted worker message to the protocol (shape checked). */
export function isHsctlWorkerResponse(
  value: unknown,
): value is HsctlWorkerResponse {
  return (
    typeof value === "object" &&
    value !== null &&
    typeof (value as { type?: unknown }).type === "string" &&
    responseTypes.has((value as { type: string }).type)
  );
}
