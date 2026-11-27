import { readFileSync } from "node:fs";

import type { Plugin } from "vite";

/*
 * Emits the public web app manifest (WEB-TERM-11) at the root of the client
 * build output as `manifest.webmanifest`, with HyperShell brand icons emitted
 * as content-hashed assets under `/assets/`. The BFF serves the file at
 * `/manifest.webmanifest` as `application/manifest+json` without a session.
 * No service worker is registered (WEB-DPLY-02).
 */

export interface WebAppManifestIcon {
  /** Absolute path of the PNG source. */
  file: string;
  name: string;
  sizes: string;
}

export interface WebAppManifestPluginOptions {
  icons: readonly WebAppManifestIcon[];
}

export const webAppManifestFileName = "manifest.webmanifest";

export function webAppManifestPlugin(
  options: WebAppManifestPluginOptions,
): Plugin {
  let isClientBuild = false;
  let iconReferences: { reference: string; sizes: string }[] = [];

  return {
    apply: "build",
    name: "hypershell-web-app-manifest",
    configResolved(config) {
      isClientBuild = !config.build.ssr;
    },
    buildStart() {
      const environment = (this as { environment?: { name: string } })
        .environment;
      const isClient =
        environment === undefined
          ? isClientBuild
          : environment.name === "client";
      iconReferences = isClient
        ? options.icons.map((icon) => ({
            reference: this.emitFile({
              name: icon.name,
              source: readFileSync(icon.file),
              type: "asset",
            }),
            sizes: icon.sizes,
          }))
        : [];
    },
    generateBundle() {
      if (iconReferences.length === 0) {
        return;
      }
      const manifest = {
        background_color: "#ffffff",
        display: "standalone",
        icons: iconReferences.map(({ reference, sizes }) => ({
          purpose: "any",
          sizes,
          src: `/${this.getFileName(reference)}`,
          type: "image/png",
        })),
        id: "/",
        name: "HyperShell",
        scope: "/",
        short_name: "HyperShell",
        start_url: "/",
        theme_color: "#ffffff",
      };
      this.emitFile({
        fileName: webAppManifestFileName,
        source: `${JSON.stringify(manifest, null, 2)}\n`,
        type: "asset",
      });
    },
  };
}
