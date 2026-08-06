import eslint from "@eslint/js";
import globals from "globals";
import tseslint from "typescript-eslint";

import {
  rawDiagnosticRules,
  sdkImportRule,
  serverApplicationRules,
  serverSdkAndTelemetryImportRule,
  serverTelemetryImportRule,
} from "../eslint.architecture.mjs";

export default tseslint.config(
  { ignores: ["dist/**", "node_modules/**", "public/**"] },
  eslint.configs.recommended,
  ...tseslint.configs.strictTypeChecked,
  ...tseslint.configs.stylisticTypeChecked,
  {
    files: ["**/*.{cjs,js,mjs}"],
    extends: [tseslint.configs.disableTypeChecked],
  },
  {
    files: ["**/*.ts"],
    languageOptions: {
      globals: {
        ...globals.node,
      },
      parserOptions: {
        project: ["./tsconfig.json", "./tsconfig.test.json"],
        tsconfigRootDir: import.meta.dirname,
      },
    },
    rules: {
      ...rawDiagnosticRules,
      "@typescript-eslint/consistent-type-imports": "error",
    },
  },
  {
    files: ["src/**/*.ts"],
    ignores: ["src/adapters/api/**", "src/composition/**"],
    rules: { "no-restricted-imports": sdkImportRule },
  },
  {
    files: ["src/**/*.ts"],
    ignores: [
      "src/adapters/observability/**",
      "src/composition/**",
      "src/index.ts",
    ],
    rules: { "no-restricted-imports": serverSdkAndTelemetryImportRule },
  },
  {
    files: ["src/adapters/api/**/*.ts"],
    rules: { "no-restricted-imports": serverTelemetryImportRule },
  },
  {
    files: ["src/adapters/observability/**/*.ts"],
    rules: { "no-restricted-imports": sdkImportRule },
  },
  {
    files: ["src/composition/**/*.ts"],
    rules: { "no-restricted-imports": "off" },
  },
  {
    files: ["src/application/**/*.ts", "src/domain/**/*.ts"],
    rules: serverApplicationRules,
  },
);
