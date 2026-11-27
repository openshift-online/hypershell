import { mkdirSync, mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import { describe, expect, it } from "vitest";

import {
  checkBuildAssets,
  findBuildAssetProblems,
} from "./check-build-assets.mjs";

const manifest = {
  display: "standalone",
  icons: [
    { sizes: "180x180", src: "/assets/favicon-180-abc.png" },
    { sizes: "1080x1080", src: "/assets/logo-def.png" },
  ],
  name: "HyperShell",
  scope: "/",
  start_url: "/",
};
const assets = [
  "favicon-180-abc.png",
  "logo-def.png",
  "hsctl-worker-D45iTzjz.js",
  "hsctl-BRhYl4dD.wasm",
  "hsctl-BRhYl4dD.wasm.br",
];

describe("build asset check", () => {
  // Verifies: WEB-TERM-10, WEB-TERM-11
  it("accepts one worker asset and a manifest with /assets/ icons", () => {
    expect(
      findBuildAssetProblems({
        assetNames: assets,
        manifestText: JSON.stringify(manifest),
      }),
    ).toEqual([]);
  });

  // Verifies: WEB-TERM-10
  it("rejects a missing, duplicated, or renamed worker asset", () => {
    const text = JSON.stringify(manifest);
    const others = assets.filter((name) => !name.startsWith("hsctl-worker"));
    expect(
      findBuildAssetProblems({ assetNames: others, manifestText: text })[0],
    ).toMatch(/exactly one .* found 0/u);
    expect(
      findBuildAssetProblems({
        assetNames: [...assets, "hsctl-worker-Other_1.js"],
        manifestText: text,
      })[0],
    ).toMatch(/found 2: hsctl-worker-D45iTzjz\.js, hsctl-worker-Other_1\.js/u);
    expect(
      findBuildAssetProblems({
        assetNames: [
          ...others,
          "hsctl-worker.D45.js",
          "assets-hsctl-worker-x.js",
        ],
        manifestText: text,
      }),
    ).toHaveLength(1);
  });

  // Verifies: WEB-TERM-11
  it("rejects a missing or invalid manifest and icons outside /assets/", () => {
    expect(
      findBuildAssetProblems({ assetNames: assets, manifestText: null }),
    ).toEqual(["manifest.webmanifest is missing from the client build root"]);
    expect(
      findBuildAssetProblems({ assetNames: assets, manifestText: "{" }),
    ).toEqual(["manifest.webmanifest is not valid JSON"]);
    expect(
      findBuildAssetProblems({
        assetNames: assets,
        manifestText: JSON.stringify({
          ...manifest,
          display: "browser",
          icons: [
            { src: "https://cdn.example.test/icon.png" },
            { src: "/assets/missing.png" },
            { src: "/icon.png" },
          ],
          start_url: "/gateways",
        }),
      }),
    ).toEqual([
      'manifest.webmanifest must set name "HyperShell" and display "standalone"',
      'manifest.webmanifest must set start_url and scope to "/"',
      'manifest icon "https://cdn.example.test/icon.png" is not served from /assets/',
      "manifest icon /assets/missing.png is not in the build",
      'manifest icon "/icon.png" is not served from /assets/',
    ]);
    expect(
      findBuildAssetProblems({
        assetNames: assets,
        manifestText: JSON.stringify({ ...manifest, icons: [] }),
      }),
    ).toEqual(["manifest.webmanifest lists no icons"]);
  });

  // Verifies: WEB-TERM-14
  it("reads a client build directory", async () => {
    const client = mkdtempSync(path.join(tmpdir(), "client-build-"));
    mkdirSync(path.join(client, "assets"));
    for (const name of assets) {
      writeFileSync(path.join(client, "assets", name), "x");
    }
    await expect(checkBuildAssets(client)).resolves.toEqual([
      "manifest.webmanifest is missing from the client build root",
    ]);
    writeFileSync(
      path.join(client, "manifest.webmanifest"),
      JSON.stringify(manifest),
    );
    await expect(checkBuildAssets(client)).resolves.toEqual([]);
  });
});
