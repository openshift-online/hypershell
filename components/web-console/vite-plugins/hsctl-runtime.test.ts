import { mkdirSync, mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import { describe, expect, it } from "vitest";

import {
  defaultHsctlRuntimeDir,
  hsctlRuntimeModuleId,
  hsctlRuntimePlugin,
  type HsctlRuntimePluginOptions,
} from "./hsctl-runtime";

type Hook = (...args: unknown[]) => unknown;

function loadModule(
  options: HsctlRuntimePluginOptions,
  config: { command: "build" | "serve"; root: string },
  ssr = false,
): unknown {
  const plugin = hsctlRuntimePlugin(options);
  (plugin.configResolved as Hook).call({}, config);
  const resolved = (plugin.resolveId as Hook).call({}, hsctlRuntimeModuleId);
  const context = {
    addWatchFile: () => undefined,
    error: (message: string) => {
      throw new Error(message);
    },
  };
  return (plugin.load as Hook).call(context, resolved, { ssr });
}

function runtimeDir(withFiles: boolean): string {
  const root = mkdtempSync(path.join(tmpdir(), "hsctl-runtime-"));
  const dir = path.join(root, ".hsctl-runtime");
  mkdirSync(dir);
  if (withFiles) {
    writeFileSync(path.join(dir, "hsctl.wasm"), "wasm");
    writeFileSync(path.join(dir, "wasm_exec.js"), "exec");
  }
  return root;
}

describe("hsctl runtime plugin", () => {
  // Verifies: CLI-WASM-05
  it("imports both runtime files as non-inlined asset URLs", () => {
    const root = runtimeDir(true);
    const code = String(
      loadModule({ environment: {} }, { command: "build", root }),
    );
    expect(code).toContain(
      `${path.join(root, ".hsctl-runtime", "hsctl.wasm")}?url&no-inline`,
    );
    expect(code).toContain("wasm_exec.js?url&no-inline");
  });

  // Verifies: WEB-TERM-09
  it("exports null for the default directory when the files are absent", () => {
    const root = runtimeDir(false);
    for (const command of ["build", "serve"] as const) {
      expect(loadModule({ environment: {} }, { command, root })).toBe(
        "export const hsctlRuntimeAssets = null;\n",
      );
    }
  });

  // Verifies: CLI-WASM-05
  it("fails a client build when an explicit HSCTL_RUNTIME_DIR lacks the files", () => {
    const root = runtimeDir(false);
    const environment = {
      HSCTL_RUNTIME_DIR: path.join(root, ".hsctl-runtime"),
    };
    expect(() =>
      loadModule({ environment }, { command: "build", root }),
    ).toThrow(
      /HSCTL_RUNTIME_DIR is set to .*hsctl\.wasm or wasm_exec\.js is missing/u,
    );
    // The dev server and the prerender build stay lenient.
    expect(loadModule({ environment }, { command: "serve", root })).toBe(
      "export const hsctlRuntimeAssets = null;\n",
    );
    expect(loadModule({ environment }, { command: "build", root }, true)).toBe(
      "export const hsctlRuntimeAssets = null;\n",
    );
  });

  // Verifies: WEB-TERM-14
  it("always exports null when stubbed for Storybook and tests", () => {
    const root = runtimeDir(true);
    expect(
      loadModule({ environment: {}, stub: true }, { command: "build", root }),
    ).toBe("export const hsctlRuntimeAssets = null;\n");
  });

  // Verifies: CLI-WASM-05
  it("resolves the runtime directory from the environment or the default", () => {
    expect(defaultHsctlRuntimeDir("/console", {})).toBe(
      path.join("/console", ".hsctl-runtime"),
    );
    expect(
      defaultHsctlRuntimeDir("/console", { HSCTL_RUNTIME_DIR: "  " }),
    ).toBe(path.join("/console", ".hsctl-runtime"));
    expect(
      defaultHsctlRuntimeDir("/console", { HSCTL_RUNTIME_DIR: "/opt/runtime" }),
    ).toBe("/opt/runtime");
  });
});
