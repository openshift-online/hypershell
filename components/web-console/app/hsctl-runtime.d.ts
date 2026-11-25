/**
 * Virtual module produced by `vite-plugins/hsctl-runtime.ts`: the URLs of the
 * content-hashed `hsctl` browser runtime assets (CLI-WASM-05), or null when
 * this build has no runtime (the terminal then shows its load error).
 */
declare module "virtual:hsctl-runtime" {
  export interface HsctlRuntimeAssets {
    readonly wasmExecUrl: string;
    readonly wasmUrl: string;
  }

  export const hsctlRuntimeAssets: HsctlRuntimeAssets | null;
}
