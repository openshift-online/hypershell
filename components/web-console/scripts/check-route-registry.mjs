import { readdir, readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const scriptRoot = path.dirname(fileURLToPath(import.meta.url));
const webConsoleRoot = path.resolve(scriptRoot, "..");
const sharedRoutesPath = path.resolve(webConsoleRoot, "shared/routes.ts");
const routeContractPath = path.resolve(webConsoleRoot, "route-contract.json");
const productionRoots = [
  path.resolve(webConsoleRoot, "app"),
  path.resolve(webConsoleRoot, "bff/src"),
  path.resolve(webConsoleRoot, "../../packages/gateway-management-ui/src"),
  path.resolve(webConsoleRoot, "../../packages/operational-dashboard-ui/src"),
];
const excludedPatterns = [
  /\.test\.[cm]?[jt]sx?$/u,
  /\.stories\.[cm]?[jt]sx?$/u,
  /(^|\/)fixtures\//u,
];
const hardcodedRoutePattern =
  /(["'`])\/(?:gateways|dashboard|metrics|login)(?:\/[^"'`]*)?\1|to=["']\/["']|href=["']\/["']|navigate\(["']\/["']\)/u;
const productionExtensions = new Set([".ts", ".tsx", ".mjs"]);

function parseSharedRoutes(source) {
  const match = /export const routes = \{([\s\S]*?)\} as const;/u.exec(source);
  if (!match) {
    throw new Error("Shared route registry could not be parsed");
  }
  return Object.fromEntries(
    match[1]
      .split("\n")
      .map((line) => line.trim())
      .filter((line) => line.length > 0)
      .map((line) => {
        const entry = /^([A-Za-z0-9]+):\s*"([^"]+)",?$/u.exec(line);
        if (!entry) {
          throw new Error(`Shared route registry entry is invalid: ${line}`);
        }
        return [entry[1], entry[2]];
      }),
  );
}

export async function checkRouteContract() {
  const sharedRoutes = parseSharedRoutes(
    await readFile(sharedRoutesPath, "utf8"),
  );
  const contract = JSON.parse(await readFile(routeContractPath, "utf8"));
  const contractRoutes = Object.fromEntries(
    Object.entries(contract).filter(
      ([key]) => key !== "directNavigationExamples",
    ),
  );
  const expected = Object.keys(sharedRoutes).sort().join(", ");
  const actual = Object.keys(contractRoutes).sort().join(", ");
  if (expected !== actual) {
    throw new Error(`Route contract keys do not match registry: ${actual}`);
  }
  for (const [name, value] of Object.entries(sharedRoutes)) {
    if (contractRoutes[name] !== value) {
      throw new Error(
        `Route contract value for ${name} does not match registry`,
      );
    }
  }
  const examples = contract.directNavigationExamples;
  if (
    !Array.isArray(examples) ||
    !examples.every((value) => typeof value === "string")
  ) {
    throw new Error("Route contract direct navigation examples are invalid");
  }
}

async function filesIn(root) {
  const entries = await readdir(root, { withFileTypes: true, recursive: true });
  return entries
    .filter((entry) => entry.isFile())
    .map((entry) => path.join(entry.parentPath, entry.name));
}

export async function checkRouteRegistryUsage() {
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
      if (hardcodedRoutePattern.test(source)) {
        violations.push(file);
      }
    }
  }
  if (violations.length > 0) {
    throw new Error(
      `Production routes must use the shared route registry: ${violations.join(", ")}`,
    );
  }
}

export async function checkRouteRegistry() {
  await checkRouteContract();
  await checkRouteRegistryUsage();
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  await checkRouteRegistry();
  console.log("Route registry contract and usage checks passed.");
}
