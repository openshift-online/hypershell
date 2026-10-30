import { readdir, readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const scriptRoot = path.dirname(fileURLToPath(import.meta.url));
const webConsoleRoot = path.resolve(scriptRoot, "..");
const registryPath = path.resolve(
  webConsoleRoot,
  "../../packages/gateway-management-ui/src/application/external-links.json",
);
const productionRoots = [
  path.resolve(webConsoleRoot, "app"),
  path.resolve(webConsoleRoot, "../../packages/gateway-management-ui/src"),
  path.resolve(webConsoleRoot, "../../packages/operational-dashboard-ui/src"),
];
const externalUrlPattern = /(["'`])https:\/\//u;
const productionExtensions = new Set([".ts", ".tsx", ".mjs"]);
const excludedPatterns = [
  /\.test\.[cm]?[jt]sx?$/u,
  /\.stories\.[cm]?[jt]sx?$/u,
  /(^|\/)fixtures\//u,
];

export async function readExternalLinkRegistry(filePath = registryPath) {
  const parsed = JSON.parse(await readFile(filePath, "utf8"));
  if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
    throw new Error("External link registry must be a JSON object");
  }
  return parsed;
}

export function validateExternalLinkRegistry(registry) {
  const entries = Object.entries(registry);
  if (entries.length === 0) {
    throw new Error("External link registry must not be empty");
  }
  const seen = new Set();
  for (const [name, value] of entries) {
    if (!name || typeof value !== "string") {
      throw new Error(`External link registry entry ${name} is invalid`);
    }
    let url;
    try {
      url = new URL(value);
    } catch {
      throw new Error(`External link registry entry ${name} is not a URL`);
    }
    if (url.protocol !== "https:" || url.username || url.password) {
      throw new Error(
        `External link registry entry ${name} must be an HTTPS URL without credentials`,
      );
    }
    if (seen.has(value)) {
      throw new Error(`External link registry contains duplicate URL ${value}`);
    }
    seen.add(value);
  }
}

async function filesIn(root) {
  const entries = await readdir(root, { withFileTypes: true, recursive: true });
  return entries
    .filter((entry) => entry.isFile())
    .map((entry) => path.join(entry.parentPath, entry.name));
}

export async function checkExternalLinkRegistryUsage() {
  const violations = [];
  for (const root of productionRoots) {
    for (const file of await filesIn(root)) {
      if (
        !productionExtensions.has(path.extname(file)) ||
        excludedPatterns.some((pattern) => pattern.test(file))
      ) {
        continue;
      }
      const source = await readFile(file, "utf8");
      if (externalUrlPattern.test(source)) {
        violations.push(file);
      }
    }
  }
  if (violations.length > 0) {
    throw new Error(
      `Production HTTPS URLs must use the external link registry: ${violations.join(", ")}`,
    );
  }
}

async function checkUrl(name, url, fetchImplementation, timeoutMs) {
  const hostname = new URL(url).hostname;
  if (
    hostname === "localhost" ||
    hostname === "127.0.0.1" ||
    hostname === "::1" ||
    hostname.endsWith(".localhost") ||
    hostname.endsWith(".local")
  ) {
    return { name, ok: true, status: undefined, url, skipped: true };
  }
  const headers = {
    "user-agent":
      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126 Safari/537.36",
  };
  const request = (method) =>
    fetchImplementation(url, {
      headers,
      method,
      redirect: "follow",
      signal: AbortSignal.timeout(timeoutMs),
    });

  try {
    let response = await request("HEAD");
    if (response.status === 405) {
      response = await request("GET");
    }
    if (response.status >= 200 && response.status < 400) {
      return { name, ok: true, status: response.status, url };
    }
    return {
      name,
      ok: false,
      status: response.status,
      url,
      error: `HTTP ${String(response.status)}`,
    };
  } catch (error) {
    return {
      name,
      ok: false,
      status: undefined,
      url,
      error: error instanceof Error ? error.message : "network error",
    };
  }
}

export async function checkExternalLinks({
  concurrency = 4,
  fetchImplementation = globalThis.fetch,
  registry,
  timeoutMs = 10_000,
}) {
  validateExternalLinkRegistry(registry);
  const entries = Object.entries(registry);
  const results = [];
  for (let offset = 0; offset < entries.length; offset += concurrency) {
    results.push(
      ...(await Promise.all(
        entries
          .slice(offset, offset + concurrency)
          .map(([name, url]) =>
            checkUrl(name, url, fetchImplementation, timeoutMs),
          ),
      )),
    );
  }
  return results;
}

async function main() {
  const staticOnly = process.argv.includes("--static");
  const registry = await readExternalLinkRegistry();
  validateExternalLinkRegistry(registry);
  await checkExternalLinkRegistryUsage();
  if (staticOnly) {
    console.log("External link registry integrity and usage checks passed.");
    return;
  }

  const results = await checkExternalLinks({ registry });
  for (const result of results) {
    const status = result.skipped ? "SKIP" : result.ok ? "PASS" : "FAIL";
    const detail = result.ok
      ? String(result.status ?? "")
      : (result.error ?? "unknown failure");
    console.log(`${status} ${result.name} ${result.url} ${detail}`);
  }
  const failures = results.filter((result) => !result.ok).length;
  console.log(
    `External links: ${String(results.length - failures)} passed, ${String(failures)} failed.`,
  );
  if (failures > 0) {
    process.exitCode = 1;
  }
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  await main();
}
