import path from "node:path";
import { defineConfig } from "vite";

import { specsPlugin } from "./server/specs-plugin";

// SPEC_VIEWER_SPECS_DIR points the viewer at another specs tree; relative
// paths resolve from this component's directory.
const specsRoot = path.resolve(
  import.meta.dirname,
  process.env.SPEC_VIEWER_SPECS_DIR ?? "../../specs",
);

export default defineConfig({
  base: "./",
  plugins: [specsPlugin(specsRoot)],
  // PatternFly dominates the main chunk; shiki languages load on demand.
  build: { chunkSizeWarningLimit: 1000 },
  server: {
    host: "127.0.0.1",
    port: Number(process.env.SPEC_VIEWER_PORT ?? "5180"),
    // Pins live in localStorage, which is per origin: a silent fallback to
    // another port would start with no pins.
    strictPort: true,
  },
});
