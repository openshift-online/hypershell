// Post-build check for the browser assets the BFF serves specially
// (WEB-TERM-10, WEB-TERM-11, WEB-TERM-14). The BFF gives the hsctl worker its
// own CSP by matching /assets/hsctl-worker-<hash>.js, and serves
// manifest.webmanifest without a session, so a renamed worker or a missing or
// malformed manifest would silently break the terminal or the installed app.
import { existsSync } from "node:fs";
import { readdir, readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

/** Must stay in sync with the BFF worker route pattern. */
export const workerAssetPattern = /^hsctl-worker-[A-Za-z0-9_-]+\.js$/u;
const iconSourcePattern = /^\/assets\/([^/?#]+)$/u;

/**
 * Returns every problem found in a client build, given its asset file names
 * and the manifest text (or null when the manifest is missing).
 */
export function findBuildAssetProblems({ assetNames, manifestText }) {
  const problems = [];
  const workers = assetNames.filter((name) => workerAssetPattern.test(name));
  if (workers.length !== 1) {
    problems.push(
      `expected exactly one assets/hsctl-worker-<hash>.js matching ${String(workerAssetPattern)}, found ${String(workers.length)}${workers.length > 0 ? `: ${workers.join(", ")}` : ""}`,
    );
  }

  if (manifestText === null) {
    problems.push("manifest.webmanifest is missing from the client build root");
    return problems;
  }
  let manifest;
  try {
    manifest = JSON.parse(manifestText);
  } catch {
    problems.push("manifest.webmanifest is not valid JSON");
    return problems;
  }
  if (manifest?.name !== "HyperShell" || manifest.display !== "standalone") {
    problems.push(
      'manifest.webmanifest must set name "HyperShell" and display "standalone"',
    );
  }
  if (manifest.start_url !== "/" || manifest.scope !== "/") {
    problems.push('manifest.webmanifest must set start_url and scope to "/"');
  }
  const icons = Array.isArray(manifest.icons) ? manifest.icons : [];
  if (icons.length === 0) {
    problems.push("manifest.webmanifest lists no icons");
  }
  const assetSet = new Set(assetNames);
  for (const icon of icons) {
    const source = typeof icon?.src === "string" ? icon.src : "";
    const match = iconSourcePattern.exec(source);
    if (match === null) {
      problems.push(
        `manifest icon ${JSON.stringify(source)} is not served from /assets/`,
      );
    } else if (!assetSet.has(match[1])) {
      problems.push(`manifest icon ${source} is not in the build`);
    }
  }
  return problems;
}

/** Reads a client build directory and checks it. */
export async function checkBuildAssets(clientDirectory) {
  const assetNames = await readdir(path.join(clientDirectory, "assets"));
  const manifestPath = path.join(clientDirectory, "manifest.webmanifest");
  const manifestText = existsSync(manifestPath)
    ? await readFile(manifestPath, "utf8")
    : null;
  return findBuildAssetProblems({ assetNames, manifestText });
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const webConsoleRoot = path.dirname(
    path.dirname(fileURLToPath(import.meta.url)),
  );
  const clientDirectory =
    process.argv[2] ?? path.join(webConsoleRoot, "build", "client");
  const problems = await checkBuildAssets(clientDirectory);
  if (problems.length > 0) {
    for (const problem of problems) {
      console.error(`check-build-assets: ${problem}`);
    }
    process.exitCode = 1;
  } else {
    console.log("check-build-assets: worker asset and web app manifest OK");
  }
}
