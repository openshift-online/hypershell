import { loader } from "@monaco-editor/react";
import * as monaco from "monaco-editor/editor/editor.api";
import EditorWorker from "monaco-editor/editor/editor.worker?worker";

/*
 * Client-only Monaco setup for the Cedar editor. Monaco is bundled with the
 * console and handed to the loader, so the editor never fetches Monaco from a
 * CDN (no third-party request, CSP-friendly, deterministic in Storybook). Only
 * the core editor API and its base worker are included: Cedar needs no
 * language service, so no TypeScript, JSON, CSS, or HTML workers ship.
 */

export const cedarLanguageId = "cedar";

const monacoEnvironment = {
  getWorker: () => new EditorWorker(),
};

Object.assign(globalThis, { MonacoEnvironment: monacoEnvironment });

monaco.languages.register({ id: cedarLanguageId });
monaco.languages.setLanguageConfiguration(cedarLanguageId, {
  brackets: [
    ["{", "}"],
    ["[", "]"],
    ["(", ")"],
  ],
  comments: { lineComment: "//" },
  autoClosingPairs: [
    { close: "}", open: "{" },
    { close: "]", open: "[" },
    { close: ")", open: "(" },
    { close: '"', notIn: ["string"], open: '"' },
  ],
});
monaco.languages.setMonarchTokensProvider(cedarLanguageId, {
  keywords: [
    "forbid",
    "permit",
    "when",
    "unless",
    "if",
    "then",
    "else",
    "in",
    "has",
    "like",
    "is",
  ],
  variables: ["principal", "action", "resource", "context"],
  literals: ["true", "false"],
  tokenizer: {
    root: [
      [/\/\/.*$/u, "comment"],
      [/@[A-Za-z_]\w*/u, "annotation"],
      [/"/u, "string", "@string"],
      [/\d+/u, "number"],
      [/[A-Z][\w]*(?=::)/u, "type"],
      [
        /[A-Za-z_]\w*/u,
        {
          cases: {
            "@keywords": "keyword",
            "@literals": "keyword",
            "@variables": "variable",
            "@default": "identifier",
          },
        },
      ],
      [/::/u, "delimiter"],
      [/&&|\|\||==|!=|<=|>=|[<>!]/u, "operator"],
      [/[{}()[\]]/u, "@brackets"],
      [/[;,.]/u, "delimiter"],
    ],
    string: [
      [/[^\\"]+/u, "string"],
      [/\\./u, "string.escape"],
      [/"/u, "string", "@pop"],
    ],
  },
});

loader.config({ monaco });

export { monaco };
