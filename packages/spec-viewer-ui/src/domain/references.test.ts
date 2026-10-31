import { sourceLanguage } from "./references";

describe("sourceLanguage", () => {
  it.each([
    ["components/control-plane/main.go", "go"],
    ["app/a.ts", "typescript"],
    ["app/a.tsx", "tsx"],
    ["vite.config.mjs", "javascript"],
    ["deploy/base/kustomization.yaml", "yaml"],
    [".github/workflows/checks.yml", "yaml"],
    ["package.json", "json"],
    ["specs/index.spec.md", "markdown"],
    ["scripts/kind/up.sh", "shellscript"],
    ["Makefile", "make"],
    ["components/web-console/Dockerfile", "dockerfile"],
    ["images/Dockerfile.base", "dockerfile"],
    ["LICENSE", "text"],
    ["data.unknown", "text"],
  ])("colors %s as %s", (file, language) => {
    expect(sourceLanguage(file)).toBe(language);
  });
});
