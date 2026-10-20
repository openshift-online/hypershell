/// <reference types="vite/client" />

interface ImportMetaEnv {
  /**
   * DEV-only override (milliseconds) for the fleet + promotion refetch cadence. Unset
   * in normal and production builds, where the cadence in query/hooks.ts applies as-is;
   * the local mock dev server sets it so value-change animations can be eyeballed
   * without waiting out the real poll intervals.
   */
  readonly VITE_FAST_POLL_MS?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
