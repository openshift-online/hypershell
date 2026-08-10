import eslint from "@eslint/js";
import formatjs from "eslint-plugin-formatjs";
import jsxA11y from "eslint-plugin-jsx-a11y";
import reactCompiler from "eslint-plugin-react-compiler";
import reactHooks from "eslint-plugin-react-hooks";
import globals from "globals";
import query from "@tanstack/eslint-plugin-query";
import tseslint from "typescript-eslint";

export default tseslint.config(
  {
    ignores: ["coverage/**", "node_modules/**"],
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
    files: ["src/**/*.{ts,tsx}"],
    rules: {
      "no-console": "error",
      "no-restricted-imports": [
        "error",
        {
          paths: [
            {
              name: "@openshift-online/hypershell-sdk",
              message: "Use the application-owned GatewayControlPlane port.",
            },
            {
              name: "@openshift-online/hypershell-domain-probes/fan-out",
              message: "Keep concrete probe fan-out in the host adapter.",
            },
          ],
          patterns: [
            {
              group: ["@opentelemetry/*"],
              message: "Publish typed gateway facts through the probe port.",
            },
          ],
        },
      ],
    },
  },
  {
    files: ["src/messages.ts"],
    rules: {
      "sort-keys": [
        "error",
        "asc",
        { caseSensitive: false, minKeys: 4, natural: true },
      ],
    },
  },
  {
    files: ["src/application/**/*.ts"],
    rules: {
      "no-restricted-globals": [
        "error",
        ...[
          "document",
          "navigator",
          "window",
          "localStorage",
          "sessionStorage",
          "fetch",
        ].map((name) => ({
          name,
          message: "Keep external capabilities behind application-owned ports.",
        })),
      ],
      "no-restricted-imports": [
        "error",
        {
          paths: [
            ...[
              "@patternfly/react-core",
              "@tanstack/react-query",
              "react",
              "react-dom",
              "react-router",
            ].map((name) => ({
              name,
              message: "Keep UI frameworks in presentation adapters.",
            })),
            {
              name: "@openshift-online/hypershell-sdk",
              message: "Use the application-owned GatewayControlPlane port.",
            },
            {
              name: "@openshift-online/hypershell-domain-probes/fan-out",
              message: "Keep concrete probe fan-out in the host adapter.",
            },
          ],
          patterns: [
            {
              group: [
                "@opentelemetry/*",
                "@patternfly/*",
                "@tanstack/*",
                "**/adapters/**",
              ],
              message: "Application dependencies must point inward.",
            },
          ],
        },
      ],
      "no-restricted-syntax": [
        "error",
        {
          selector: "NewExpression[callee.name='Date']",
          message: "Read time through GatewayWorkflowRuntime.",
        },
        {
          selector:
            "CallExpression[callee.object.name='Date'][callee.property.name='now']",
          message: "Read time through GatewayWorkflowRuntime.",
        },
        {
          selector:
            "CallExpression[callee.object.name='Math'][callee.property.name='random']",
          message: "Read randomness through GatewayWorkflowRuntime.",
        },
      ],
    },
  },
);
