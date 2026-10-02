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
    ignores: ["coverage/**", "dist/**", "node_modules/**"],
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
  // --- Hexagonal boundaries (mirrors components/web-console/eslint.architecture.mjs) ---
  // The domain + application layers are framework-free and depend only inward.
  {
    files: ["src/domain/**/*.ts", "src/application/**/*.ts"],
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
            "@patternfly/react-core",
            "@patternfly/react-icons",
            "@patternfly/react-table",
            "@tanstack/react-query",
            "react",
            "react-dom",
            "react-intl",
          ].map((name) => ({
            name,
            message: "Keep UI frameworks in presentation/adapter layers.",
          })),
          patterns: [
            {
              group: ["@patternfly/*", "@tanstack/*", "**/adapters/**"],
              message: "Domain and application dependencies must point inward.",
            },
          ],
        },
      ],
      "no-restricted-syntax": [
        "error",
        {
          selector: "NewExpression[callee.name='Date']",
          message: "Read the clock through an application-owned port.",
        },
        {
          selector:
            "CallExpression[callee.object.name='Date'][callee.property.name='now']",
          message: "Read the clock through an application-owned port.",
        },
        {
          selector:
            "CallExpression[callee.object.name='Math'][callee.property.name='random']",
          message: "Read randomness through an application-owned port.",
        },
      ],
    },
  },
);
