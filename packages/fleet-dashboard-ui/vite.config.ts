import { defineConfig } from "vite";

// No @vitejs/plugin-react: Vite's built-in esbuild transforms .tsx using the
// tsconfig `jsx: react-jsx` setting, which is sufficient for a production build.
// (Fast Refresh / the React Compiler babel pass are dev niceties we forgo to
// avoid adding a dependency; the React Compiler lint rules still run.)
export default defineConfig({
  build: {
    outDir: "dist",
    // The Go BFF serves this bundle from an embedded FS with an SPA fallback.
    sourcemap: false,
  },
  server: {
    // `pnpm dev` proxies API calls to a locally running BFF (see the BFF's
    // default dev port). No fleet identity is encoded here - only a loopback.
    proxy: {
      "/api": "http://localhost:8081",
    },
  },
});
