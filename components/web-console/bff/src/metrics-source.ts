import { readFile } from "node:fs/promises";
import { request } from "node:https";

const metricsResponseLimit = 4 * 1024 * 1024;

export type MetricsSource =
  | string
  | {
      url: string;
      tokenFile?: string;
      caFile?: string;
    };

async function readCredential(
  file: string,
  variable: "CLUSTER_PROMETHEUS_TOKEN_FILE" | "CLUSTER_PROMETHEUS_CA_FILE",
): Promise<string> {
  let value: string;
  try {
    value = (await readFile(file, "utf8")).trim();
  } catch (error) {
    const missing =
      error instanceof Error && "code" in error && error.code === "ENOENT";
    // Fixed labels identify the mount and failure without logging paths, values,
    // or the original filesystem error. Route handlers log this server-side.
    throw new Error(
      `Could not read metrics credentials: ${variable} is ${missing ? "missing" : "unreadable"}`,
    );
  }
  if (!value) {
    throw new Error(`Could not read metrics credentials: ${variable} is empty`);
  }
  if (
    variable === "CLUSTER_PROMETHEUS_TOKEN_FILE" &&
    !/^[a-z0-9._~+/=-]+$/iu.test(value)
  ) {
    throw new Error(
      `Could not read metrics credentials: ${variable} has an invalid token format`,
    );
  }
  return value;
}

async function readBoundedResponse(response: Response): Promise<Response> {
  if (!response.ok) {
    await response.body?.cancel();
    return response;
  }
  const reader = response.body?.getReader() as
    ReadableStreamDefaultReader<Uint8Array> | undefined;
  if (!reader) return response;
  const chunks: Buffer[] = [];
  let size = 0;
  try {
    for (
      let part = await reader.read();
      !part.done;
      part = await reader.read()
    ) {
      const value = part.value;
      size += value.byteLength;
      if (size > metricsResponseLimit) {
        throw new Error("Metrics response too large");
      }
      chunks.push(Buffer.from(value));
    }
  } finally {
    try {
      await reader.cancel();
    } finally {
      reader.releaseLock();
    }
  }
  return new Response(Buffer.concat(chunks).toString("utf8"), {
    status: response.status,
  });
}

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
    return readBoundedResponse(await fetch(url, { signal, redirect: "error" }));
  }
  if (url.protocol !== "https:" || url.username || url.password) {
    throw new Error(
      "Authenticated metrics require HTTPS without URL credentials",
    );
  }

  // Projected files can rotate. Read them for every request, not only at startup.
  const token = settings.tokenFile
    ? await readCredential(settings.tokenFile, "CLUSTER_PROMETHEUS_TOKEN_FILE")
    : undefined;
  const ca = settings.caFile
    ? await readCredential(settings.caFile, "CLUSTER_PROMETHEUS_CA_FILE")
    : undefined;
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
          if (size > metricsResponseLimit) {
            req.destroy();
            reject(new Error("Metrics response too large"));
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
