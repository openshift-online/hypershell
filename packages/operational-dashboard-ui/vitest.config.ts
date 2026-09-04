import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    environment: "jsdom",
    globals: true,
    include: ["src/**/*.test.{ts,tsx}"],
    setupFiles: ["./vitest.setup.ts"],
    coverage: {
      provider: "v8",
      reporter: ["text", "json-summary"],
      include: [
        "src/dashboard/dashboard-layout-persistence.ts",
        "src/dashboard/gateway-status-data.ts",
        "src/dashboard/node-status-data.ts",
        "src/dashboard/pod-capacity-data.ts",
        "src/dashboard/status-donut-data.ts",
        "src/dashboard/metric-trend-change.ts",
      ],
      exclude: ["src/**/*.test.{ts,tsx}"],
      thresholds: {
        branches: 80,
        functions: 80,
        lines: 80,
        statements: 80,
      },
    },
  },
});
