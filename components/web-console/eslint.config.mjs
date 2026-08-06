import eslint from "@eslint/js";
import formatjs from "eslint-plugin-formatjs";
import jsxA11y from "eslint-plugin-jsx-a11y";
import reactCompiler from "eslint-plugin-react-compiler";
import reactHooks from "eslint-plugin-react-hooks";
import globals from "globals";
import query from "@tanstack/eslint-plugin-query";
import tseslint from "typescript-eslint";

import {
  browserApplicationRules,
  browserSdkAndTelemetryImportRule,
  browserTelemetryImportRule,
  rawDiagnosticRules,
  sdkImportRule,
} from "./eslint.architecture.mjs";

export default tseslint.config(
  {
    ignores: [
      ".react-router/**",
      "bff/**",
      "build/**",
      "coverage/**",
      "domain-probes/**",
      "node_modules/**",
      "playwright-report/**",
      "storybook-static/**",
      "test-results/**",
    ],
  },
  eslint.configs.recommended,
  ...tseslint.configs.strictTypeChecked,
  ...tseslint.configs.stylisticTypeChecked,
  {
    files: ["**/*.{cjs,js,mjs}"],
    extends: [tseslint.configs.disableTypeChecked],
  },
  {
    files: ["**/*.{ts,tsx}"],
    languageOptions: {
      globals: {
        ...globals.browser,
        ...globals.node,
      },
      parserOptions: {
        project: ["./tsconfig.app.json", "./tsconfig.test.json"],
        tsconfigRootDir: import.meta.dirname,
      },
    },
    plugins: {
      formatjs,
      "jsx-a11y": jsxA11y,
      "react-compiler": reactCompiler,
      "react-hooks": reactHooks,
      "@tanstack/query": query,
    },
    rules: {
      ...formatjs.configs.recommended.rules,
      ...jsxA11y.flatConfigs.recommended.rules,
      ...reactCompiler.configs.recommended.rules,
      ...reactHooks.configs.flat.recommended.rules,
      ...query.configs["flat/recommended"].rules,
      "@typescript-eslint/consistent-type-imports": "error",
      "formatjs/enforce-default-message": "error",
      "formatjs/enforce-id": "error",
    },
  },
  {
    files: ["app/**/*.{ts,tsx}"],
    rules: rawDiagnosticRules,
  },
  {
    files: ["app/**/*.{ts,tsx}"],
    ignores: ["app/adapters/api/**", "app/composition/**"],
    rules: { "no-restricted-imports": sdkImportRule },
  },
  {
    files: ["app/**/*.{ts,tsx}"],
    ignores: ["app/adapters/observability/**", "app/composition/**"],
    rules: { "no-restricted-imports": browserSdkAndTelemetryImportRule },
  },
  {
    files: ["app/adapters/api/**/*.{ts,tsx}"],
    rules: { "no-restricted-imports": browserTelemetryImportRule },
  },
  {
    files: ["app/adapters/observability/**/*.{ts,tsx}"],
    rules: { "no-restricted-imports": sdkImportRule },
  },
  {
    files: ["app/composition/**/*.{ts,tsx}"],
    rules: { "no-restricted-imports": "off" },
  },
  {
    files: ["app/application/**/*.{ts,tsx}", "app/domain/**/*.{ts,tsx}"],
    rules: browserApplicationRules,
  },
);
