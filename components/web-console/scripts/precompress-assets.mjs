// Writes Brotli and gzip siblings for the large hsctl WebAssembly runtime so
// the BFF serves them precompressed (@fastify/static preCompressed) instead of
// compressing the module on every request (CLI-WASM-05).
import { readdir, readFile, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { brotliCompressSync, constants, gzipSync } from "node:zlib";

export async function precompressAssets(assetsDirectory) {
  const written = [];
  for (const name of await readdir(assetsDirectory)) {
    if (!name.endsWith(".wasm")) {
      continue;
    }
    const file = path.join(assetsDirectory, name);
    const source = await readFile(file);
    await writeFile(
      `${file}.br`,
      brotliCompressSync(source, {
        params: { [constants.BROTLI_PARAM_QUALITY]: 11 },
      }),
    );
    await writeFile(`${file}.gz`, gzipSync(source, { level: 9 }));
    written.push(name);
  }
  return written;
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const directory = process.argv[2];
  if (directory === undefined) {
    console.error("usage: precompress-assets.mjs <assets directory>");
    process.exitCode = 2;
  } else {
    const written = await precompressAssets(directory);
    console.log(
      `Precompressed ${String(written.length)} WebAssembly asset(s).`,
    );
  }
}
