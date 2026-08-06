import assert from "node:assert/strict";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { ESLint } from "eslint";

const webConsoleRoot = path.dirname(
  path.dirname(fileURLToPath(import.meta.url)),
);

function ruleSeverity(config, name) {
  const rule = config.rules?.[name];
  return Array.isArray(rule) ? rule[0] : rule;
}

function ruleText(config, name) {
  return JSON.stringify(config.rules?.[name] ?? null);
}

async function loadConfig(cwd, relativePath) {
  const eslint = new ESLint({ cwd });
  const config = await eslint.calculateConfigForFile(
    path.join(cwd, relativePath),
  );
  assert(config, `ESLint did not resolve configuration for ${relativePath}`);
  return config;
}

const browserFeature = await loadConfig(
  webConsoleRoot,
  "app/features/example.tsx",
);
assert.equal(ruleSeverity(browserFeature, "no-console"), 2);
assert.match(
  ruleText(browserFeature, "no-restricted-imports"),
  /hypershell-sdk/,
);
assert.match(
  ruleText(browserFeature, "no-restricted-imports"),
  /opentelemetry/,
);

const browserApplication = await loadConfig(
  webConsoleRoot,
  "app/application/example.ts",
);
assert.equal(ruleSeverity(browserApplication, "no-console"), 2);
assert.match(
  ruleText(browserApplication, "no-restricted-imports"),
  /react-router/,
);
assert.match(ruleText(browserApplication, "no-restricted-imports"), /tanstack/);
assert.match(ruleText(browserApplication, "no-restricted-imports"), /adapters/);
assert.match(
  ruleText(browserApplication, "no-restricted-imports"),
  /domain-probes\/fan-out/,
);
assert.match(
  ruleText(browserApplication, "no-restricted-globals"),
  /localStorage/,
);
assert.match(ruleText(browserApplication, "no-restricted-syntax"), /Math/);

const browserApiAdapter = await loadConfig(
  webConsoleRoot,
  "app/adapters/api/example.ts",
);
assert.doesNotMatch(
  ruleText(browserApiAdapter, "no-restricted-imports"),
  /hypershell-sdk/,
);
assert.match(
  ruleText(browserApiAdapter, "no-restricted-imports"),
  /opentelemetry/,
);

const browserObservabilityAdapter = await loadConfig(
  webConsoleRoot,
  "app/adapters/observability/example.ts",
);
assert.match(
  ruleText(browserObservabilityAdapter, "no-restricted-imports"),
  /hypershell-sdk/,
);
assert.doesNotMatch(
  ruleText(browserObservabilityAdapter, "no-restricted-imports"),
  /opentelemetry/,
);

const browserComposition = await loadConfig(
  webConsoleRoot,
  "app/composition/example.ts",
);
assert.equal(ruleSeverity(browserComposition, "no-restricted-imports"), 0);

const bffRoot = path.join(webConsoleRoot, "bff");
const bffApplication = await loadConfig(bffRoot, "src/application/example.ts");
assert.equal(ruleSeverity(bffApplication, "no-console"), 2);
assert.match(ruleText(bffApplication, "no-restricted-imports"), /fastify/);
assert.match(ruleText(bffApplication, "no-restricted-imports"), /node:\*/);
assert.match(
  ruleText(bffApplication, "no-restricted-imports"),
  /opentelemetry/,
);
assert.match(
  ruleText(bffApplication, "no-restricted-imports"),
  /domain-probes\/fan-out/,
);
assert.match(ruleText(bffApplication, "no-restricted-globals"), /process/);

const bffApiAdapter = await loadConfig(bffRoot, "src/adapters/api/example.ts");
assert.doesNotMatch(
  ruleText(bffApiAdapter, "no-restricted-imports"),
  /hypershell-sdk/,
);
assert.match(ruleText(bffApiAdapter, "no-restricted-imports"), /opentelemetry/);

const bffObservabilityAdapter = await loadConfig(
  bffRoot,
  "src/adapters/observability/example.ts",
);
assert.match(
  ruleText(bffObservabilityAdapter, "no-restricted-imports"),
  /hypershell-sdk/,
);
assert.doesNotMatch(
  ruleText(bffObservabilityAdapter, "no-restricted-imports"),
  /opentelemetry/,
);

const bffComposition = await loadConfig(bffRoot, "src/composition/example.ts");
assert.equal(ruleSeverity(bffComposition, "no-restricted-imports"), 0);

const probeRoot = path.join(webConsoleRoot, "domain-probes");
const probeConfig = await loadConfig(probeRoot, "src/example.ts");
assert.equal(ruleSeverity(probeConfig, "no-console"), 2);
