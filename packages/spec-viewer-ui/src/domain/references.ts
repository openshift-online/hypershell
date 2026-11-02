// Traceability: where in the repository a requirement or scenario ID appears.

export interface ReferenceMatch {
  /** 1-based line number. */
  line: number;
  text: string;
}

export interface FileReferences {
  /** Path relative to the repository root. */
  path: string;
  /** True for test files, which give a requirement its traceability. */
  isTest: boolean;
  matches: ReferenceMatch[];
}

export type SourceLanguage =
  | "go"
  | "typescript"
  | "tsx"
  | "javascript"
  | "jsx"
  | "yaml"
  | "json"
  | "markdown"
  | "shellscript"
  | "python"
  | "make"
  | "dockerfile"
  | "proto"
  | "sql"
  | "toml"
  | "css"
  | "html"
  | "text";

const BY_EXTENSION: Record<string, SourceLanguage> = {
  go: "go",
  ts: "typescript",
  mts: "typescript",
  cts: "typescript",
  tsx: "tsx",
  js: "javascript",
  mjs: "javascript",
  cjs: "javascript",
  jsx: "jsx",
  yaml: "yaml",
  yml: "yaml",
  json: "json",
  md: "markdown",
  sh: "shellscript",
  bash: "shellscript",
  py: "python",
  mk: "make",
  proto: "proto",
  sql: "sql",
  toml: "toml",
  css: "css",
  html: "html",
};

const BY_NAME: Record<string, SourceLanguage> = {
  Makefile: "make",
  Dockerfile: "dockerfile",
};

/** Syntax to color a repository file with, from its name. */
export function sourceLanguage(file: string): SourceLanguage {
  const name = file.split("/").pop() ?? file;
  const byName =
    BY_NAME[name] ?? (name.startsWith("Dockerfile") ? "dockerfile" : undefined);
  if (byName) return byName;
  const extension = name.includes(".") ? name.split(".").pop() : undefined;
  return (extension ? BY_EXTENSION[extension] : undefined) ?? "text";
}
