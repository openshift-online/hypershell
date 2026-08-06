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
);
