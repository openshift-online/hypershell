import { readFile } from "node:fs/promises";
import { request } from "node:https";

export type MetricsSource =
  | string
  | {
      url: string;
      tokenFile?: string;
      caFile?: string;
    };

/** Fetch a query without forwarding credentials to redirects or other sources. */
export async function fetchMetrics(
  source: MetricsSource,
  query: string,
  signal: AbortSignal,
): Promise<Response> {
  const settings = typeof source === "string" ? { url: source } : source;
  const url = new URL("/api/v1/query", settings.url);
  url.searchParams.set("query", query);
  if (!settings.tokenFile && !settings.caFile) {
    return fetch(url, { signal, redirect: "error" });
  }
  if (url.protocol !== "https:" || url.username || url.password) {
    throw new Error(
      "Authenticated metrics require HTTPS without URL credentials",
    );
  }

  // Projected files can rotate. Read them for every request, not only at startup.
  let token: string | undefined;
  let ca: string | undefined;
  try {
    token = settings.tokenFile
      ? (await readFile(settings.tokenFile, "utf8")).trim()
      : undefined;
    ca = settings.caFile ? await readFile(settings.caFile, "utf8") : undefined;
    if ((settings.tokenFile && !token) || (settings.caFile && !ca?.trim())) {
      throw new Error("Empty credentials");
    }
    if (token && !/^[a-z0-9._~+/=-]+$/iu.test(token)) {
      throw new Error("Invalid token file");
    }
  } catch {
    throw new Error("Could not read metrics credentials");
  }
  return new Promise<Response>((resolve, reject) => {
    const req = request(
      url,
      {
        agent: false,
        ca,
        rejectUnauthorized: true,
        signal,
        headers: token ? { Authorization: `Bearer ${token}` } : {},
      },
      (res) => {
        if (res.statusCode !== 200) {
          res.destroy();
          reject(
            new Error(
              `Metrics endpoint returned HTTP ${String(res.statusCode)}`,
            ),
          );
          return;
        }
        const chunks: Buffer[] = [];
        let size = 0;
        res.on("data", (chunk: Buffer) => {
          size += chunk.length;
          if (size > 4 * 1024 * 1024) {
            req.destroy(new Error("Metrics response too large"));
            return;
          }
          chunks.push(chunk);
        });
        res.on("error", () => {
          reject(new Error("Metrics response failed"));
        });
        res.on("end", () => {
          resolve(new Response(Buffer.concat(chunks).toString("utf8")));
        });
      },
    );
    // Do not attach the original request or credential values to logged errors.
    req.on("error", () => {
      reject(
        new Error(
          signal.aborted
            ? "Prometheus query timed out"
            : "Metrics HTTPS request failed",
        ),
      );
    });
    req.end();
  });
}

export function namespaceSelector(
  namespace?: string,
  label = "namespace",
): string {
  if (!namespace) return "";
  if (
    !/^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/u.test(namespace) ||
    namespace.length > 63
  ) {
    throw new Error("Invalid metrics namespace");
  }
  return `{${label}=${JSON.stringify(namespace)}}`;
}
