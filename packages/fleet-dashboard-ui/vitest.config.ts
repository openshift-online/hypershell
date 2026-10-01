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
      // Coverage thresholds are enforced on the framework-free logic layers
      // (domain + application). Presentation/adapters are exercised by component
      // tests but not gated on line coverage, matching how the logic-heavy layers
      // carry the correctness burden in this hexagonal package.
      include: ["src/domain/**/*.ts", "src/application/**/*.ts"],
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
