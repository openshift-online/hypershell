import { defineConfig } from "vitest/config";

import { hsctlRuntimePlugin } from "./vite-plugins/hsctl-runtime";

export default defineConfig({
  // Unit tests never load the hsctl runtime (WEB-TERM-14).
  plugins: [hsctlRuntimePlugin({ stub: true })],
  test: {
    environment: "jsdom",
    globals: true,
    include: [
      "app/**/*.test.{ts,tsx}",
      "scripts/**/*.test.mjs",
      "vite-plugins/**/*.test.ts",
    ],
    setupFiles: ["./vitest.setup.ts"],
    coverage: {
      provider: "v8",
      reporter: ["text", "json-summary"],
      include: ["app/features/**/*.tsx", "app/i18n/**/*.ts"],
      exclude: ["app/**/*.stories.tsx", "app/**/*-route.tsx"],
      thresholds: {
        branches: 80,
        functions: 80,
        lines: 80,
        statements: 80,
      },
    },
  },
});
