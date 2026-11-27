import { existsSync } from "node:fs";
import path from "node:path";

import type { Plugin } from "vite";

/*
 * Exposes the `hsctl` browser runtime (CLI-WASM-05) to the console through
 * the `virtual:hsctl-runtime` module.
 *
 * The CLI build writes `hsctl.wasm` and Go's `wasm_exec.js` into
 * HSCTL_RUNTIME_DIR (default `<web-console>/.hsctl-runtime`). When both files
 * exist, the virtual module imports them with Vite's `?url&no-inline` asset
 * handling, so a production build emits content-hashed copies
 * (`assets/hsctl-<hash>.wasm`, `assets/wasm_exec-<hash>.js`) and the dev
 * server serves them from the directory. When either file is absent (dev, CI,
 * Storybook, tests) the module exports null and the terminal shows its
 * localized load error. When HSCTL_RUNTIME_DIR is set explicitly, as the
 * image build does, a production client build without both files fails
 * instead, so an image can never ship without the runtime. The runtime
 * binaries are never committed.
 */

export const hsctlRuntimeModuleId = "virtual:hsctl-runtime";
const resolvedModuleId = `\0${hsctlRuntimeModuleId}`;

export interface HsctlRuntimePluginOptions {
  /** Directory holding `hsctl.wasm` and `wasm_exec.js`. */
  runtimeDir?: string;
  /** Always export null (Storybook and unit tests never load the runtime). */
  stub?: boolean;
  /** Environment to read HSCTL_RUNTIME_DIR from (tests). */
  environment?: Readonly<Record<string, string | undefined>>;
}

function configuredRuntimeDir(
  environment: Readonly<Record<string, string | undefined>>,
): string | undefined {
  const configured = environment.HSCTL_RUNTIME_DIR?.trim();
  return configured && configured.length > 0 ? configured : undefined;
}

export function defaultHsctlRuntimeDir(
  webConsoleRoot: string,
  environment: Readonly<Record<string, string | undefined>> = process.env,
): string {
  const configured = configuredRuntimeDir(environment);
  return configured === undefined
    ? path.join(webConsoleRoot, ".hsctl-runtime")
    : path.resolve(configured);
}

const nullModule = "export const hsctlRuntimeAssets = null;\n";

export function hsctlRuntimePlugin(
  options: HsctlRuntimePluginOptions = {},
): Plugin {
  const environment = options.environment ?? process.env;
  let root = process.cwd();
  let isBuild = false;
  return {
    name: "hypershell-hsctl-runtime",
    configResolved(config) {
      root = config.root;
      isBuild = config.command === "build";
    },
    resolveId(id) {
      return id === hsctlRuntimeModuleId ? resolvedModuleId : undefined;
    },
    load(id, loadOptions) {
      if (id !== resolvedModuleId) {
        return undefined;
      }
      // The prerender (server) build never runs the terminal.
      if (options.stub === true || loadOptions?.ssr === true) {
        return nullModule;
      }
      const runtimeDir =
        options.runtimeDir ?? defaultHsctlRuntimeDir(root, environment);
      const wasm = path.join(runtimeDir, "hsctl.wasm");
      const wasmExec = path.join(runtimeDir, "wasm_exec.js");
      if (!existsSync(wasm) || !existsSync(wasmExec)) {
        if (isBuild && configuredRuntimeDir(environment) !== undefined) {
          this.error(
            `HSCTL_RUNTIME_DIR is set to ${runtimeDir}, but hsctl.wasm or wasm_exec.js is missing there. Build the hsctl browser module first (CLI-WASM-05).`,
          );
        }
        return nullModule;
      }
      this.addWatchFile(wasm);
      this.addWatchFile(wasmExec);
      return [
        `import wasmUrl from ${JSON.stringify(`${wasm}?url&no-inline`)};`,
        `import wasmExecUrl from ${JSON.stringify(`${wasmExec}?url&no-inline`)};`,
        "export const hsctlRuntimeAssets = Object.freeze({ wasmExecUrl, wasmUrl });",
        "",
      ].join("\n");
    },
  };
}
