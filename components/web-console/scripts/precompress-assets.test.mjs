import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { brotliDecompressSync, gunzipSync } from "node:zlib";

import { afterEach, describe, expect, it } from "vitest";

import { precompressAssets } from "./precompress-assets.mjs";

describe("precompressAssets", () => {
  let directory;

  afterEach(async () => {
    await rm(directory, { force: true, recursive: true });
  });

  // Verifies: CLI-WASM-05
  it("writes Brotli and gzip siblings for WebAssembly assets only", async () => {
    directory = await mkdtemp(path.join(tmpdir(), "precompress-"));
    const wasm = Buffer.alloc(4096, 7);
    await writeFile(path.join(directory, "hsctl-abc.wasm"), wasm);
    await writeFile(path.join(directory, "app-abc.js"), "console.log(1)");

    await expect(precompressAssets(directory)).resolves.toEqual([
      "hsctl-abc.wasm",
    ]);
    expect(
      brotliDecompressSync(
        await readFile(path.join(directory, "hsctl-abc.wasm.br")),
      ),
    ).toEqual(wasm);
    expect(
      gunzipSync(await readFile(path.join(directory, "hsctl-abc.wasm.gz"))),
    ).toEqual(wasm);
    await expect(
      readFile(path.join(directory, "app-abc.js.gz")),
    ).rejects.toThrow();
  });
});
