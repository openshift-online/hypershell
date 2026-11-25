import { existsSync, readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { describe, expect, it } from "vitest";

const appRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);

// Code only docs routes use (WEB-UDOC-01). React Router loads each route module
// lazily, so this code reaches a browser only when a docs route is visited,
// unless a non-docs entry imports it statically.
const docsOnly = [
  path.join(appRoot, "features", "docs") + path.sep,
  path.join(appRoot, "application", "docs") + path.sep,
  path.join(appRoot, "composition", "docs-composition.ts"),
  path.join(appRoot, "adapters", "observability", "docs-observability.ts"),
];

const staticImport =
  /(?:^|\n)\s*(?:import|export)\s[^;]*?\sfrom\s+["'](\.[^"']+)["']|(?:^|\n)\s*import\s+["'](\.[^"']+)["']/gu;

function resolveModule(
  fromFile: string,
  specifier: string,
): string | undefined {
  const base = path.resolve(path.dirname(fromFile), specifier);
  for (const candidate of [
    base,
    `${base}.ts`,
    `${base}.tsx`,
    path.join(base, "index.ts"),
    path.join(base, "index.tsx"),
  ]) {
    if (/\.(?:ts|tsx)$/u.test(candidate) && existsSync(candidate)) {
      return candidate;
    }
  }
  return undefined;
}

/** Every app module a file reaches through static relative imports. */
function staticModuleGraph(entry: string): Set<string> {
  const seen = new Set<string>();
  const pending = [entry];
  while (pending.length > 0) {
    const file = pending.pop();
    if (file === undefined || seen.has(file)) {
      continue;
    }
    seen.add(file);
    const source = readFileSync(file, "utf8");
    for (const match of source.matchAll(staticImport)) {
      const specifier = match[1] ?? match[2];
      if (
        specifier === undefined ||
        /\.(?:css|png|svg|json)$/u.test(specifier)
      ) {
        continue;
      }
      const resolved = resolveModule(file, specifier);
      if (resolved?.startsWith(appRoot)) {
        pending.push(resolved);
      }
    }
  }
  return seen;
}

describe("docs route isolation", () => {
  // Verifies: WEB-UDOC-01, WEB-UDOC-01.02
  it("keeps docs-only modules out of the /gateways route, the shell, and the root", () => {
    const entries = [
      path.join(appRoot, "root.tsx"),
      path.join(appRoot, "routes", "application.tsx"),
      path.join(appRoot, "routes", "gateways.tsx"),
      path.join(appRoot, "routes", "home.tsx"),
    ];
    for (const entry of entries) {
      const reached = [...staticModuleGraph(entry)].filter((file) =>
        docsOnly.some((prefix) => file.startsWith(prefix)),
      );
      expect(reached, path.relative(appRoot, entry)).toEqual([]);
    }
  });

  it("follows static imports of the docs routes into docs modules", () => {
    const reached = staticModuleGraph(
      path.join(appRoot, "routes", "docs-runbook-github-jira.tsx"),
    );
    expect(
      [...reached].some((file) => file.startsWith(docsOnly[0] ?? "")),
    ).toBe(true);
  });
});
