import path from "node:path";

import { z } from "zod";

const httpUrl = z
  .url()
  .trim()
  .refine((value) => {
    const url = new URL(value);
    return (
      ["http:", "https:"].includes(url.protocol) &&
      !url.username &&
      !url.password
    );
  }, "must be an HTTP(S) URL without credentials");

const httpOrigin = z
  .url()
  .trim()
  .refine((value) => {
    const url = new URL(value);
    return (
      ["http:", "https:"].includes(url.protocol) &&
      !url.username &&
      !url.password &&
      url.pathname === "/" &&
      !url.search &&
      !url.hash
    );
  }, "must be an HTTP(S) origin without credentials, path, query, or fragment")
  .transform((value) => new URL(value).origin);

const configSchema = z.object({
  HOST: z.string().trim().min(1).default("0.0.0.0"),
  // REL-07 in specs/platform/source-release.spec.md defines this format.
  HYPERSHELL_BUILD_VERSION: z
    .string()
    .trim()
    .regex(
      /^(?:dev-[0-9a-f]{7}(?:-modified)?|v(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)-[0-9a-f]{7})$/u,
      "must contain a supported image build version",
    )
    .optional(),
  HYPERSHELL_API_ORIGIN: httpOrigin.default("http://127.0.0.1:8000"),
  HYPERSHELL_API_TIMEOUT_MS: z.coerce
    .number()
    .int()
    .min(100)
    .max(120_000)
    .default(30_000),
  LOG_LEVEL: z
    .enum(["fatal", "error", "warn", "info", "debug", "trace", "silent"])
    .default("info"),
  NODE_ENV: z.enum(["development", "test", "production"]).default("production"),
  // Tracing is optional: when the collector endpoint is absent the BFF starts
  // normally with tracing disabled and readiness unaffected (WEB-TRACE-06).
  OTEL_EXPORTER_OTLP_ENDPOINT: httpUrl.optional(),
  OTEL_SERVICE_NAME: z
    .string()
    .trim()
    .min(1)
    .default("hypershell-web-console-bff"),
  OTEL_TRACES_SAMPLE_RATIO: z.coerce.number().min(0).max(1).default(1),
  OIDC_CLIENT_ID: z.string().trim().min(1).optional(),
  OIDC_ISSUER: httpUrl.optional(),
  OIDC_POST_LOGOUT_REDIRECT_URI: httpUrl.optional(),
  OIDC_REDIRECT_URI: httpUrl.optional(),
  PORT: z.coerce.number().int().min(1).max(65_535).default(8080),
  SESSION_SECRET: z
    .string()
    .regex(/^[0-9a-f]{64}$/iu, "must be a 64-character hex string (32 bytes)")
    .optional(),
  SESSION_TTL_SECONDS: z.coerce
    .number()
    .int()
    .min(60)
    .max(86_400)
    .default(28_800),
  STATIC_ROOT: z
    .string()
    .trim()
    .min(1)
    .default(path.resolve(process.cwd(), "../build/client")),
});

/** Resolved tracing configuration, present only when a collector is configured. */
export interface TracingConfig {
  collectorEndpoint: string;
  sampleRatio: number;
  serviceName: string;
  tracesEndpoint: string;
}

export interface ServerConfig {
  apiOrigin: string;
  apiTimeoutMs: number;
  buildVersion?: string;
  host: string;
  logLevel: z.infer<typeof configSchema>["LOG_LEVEL"];
  nodeEnv: z.infer<typeof configSchema>["NODE_ENV"];
  oidcClientId?: string;
  oidcIssuer?: string;
  oidcPostLogoutRedirectUri?: string;
  oidcRedirectUri?: string;
  port: number;
  sessionSecret?: Buffer;
  sessionTtlSeconds: number;
  staticRoot: string;
  tracing?: TracingConfig;
}

/**
 * Configuration handed to the untrusted browser. This is an allowlist: only
 * values safe to reveal to a client appear here. The collector endpoint,
 * origins, session secret, and OIDC settings never cross this boundary.
 */
export interface BrowserRuntimeConfig {
  build: {
    version?: string;
  };
  tracing: {
    /**
     * Fraction of browser-rooted traces to record, 0..1. It mirrors the BFF
     * sample ratio so the browser trace root and the BFF agree on each trace,
     * and is 0 when tracing is disabled so the browser records nothing the BFF
     * cannot relay.
     */
    sampleRatio: number;
  };
}

/** Projects the server config down to the allowlist the browser may read. */
export function browserRuntimeConfig(
  config: ServerConfig,
): BrowserRuntimeConfig {
  return {
    build: config.buildVersion ? { version: config.buildVersion } : {},
    tracing: { sampleRatio: config.tracing?.sampleRatio ?? 0 },
  };
}

/** Derives the OTLP/HTTP traces URL from a collector base endpoint. */
function tracesEndpointFor(collectorEndpoint: string): string {
  return `${collectorEndpoint.replace(/\/+$/u, "")}/v1/traces`;
}

export function loadConfig(
  environment: NodeJS.ProcessEnv = process.env,
): ServerConfig {
  const result = configSchema.safeParse(environment);
  if (!result.success) {
    const problems = result.error.issues.map(
      (issue) => `${issue.path.join(".") || "configuration"}: ${issue.message}`,
    );
    throw new Error(
      `Invalid web-console BFF configuration: ${problems.join("; ")}`,
    );
  }

  if (result.data.OIDC_ISSUER) {
    const oidcProblems: string[] = [];
    if (!result.data.OIDC_CLIENT_ID) {
      oidcProblems.push("OIDC_CLIENT_ID is required when OIDC_ISSUER is set");
    }
    if (!result.data.SESSION_SECRET) {
      oidcProblems.push("SESSION_SECRET is required when OIDC_ISSUER is set");
    }
    if (oidcProblems.length > 0) {
      throw new Error(
        `Invalid web-console BFF configuration: ${oidcProblems.join("; ")}`,
      );
    }
  }

  return {
    apiOrigin: result.data.HYPERSHELL_API_ORIGIN,
    apiTimeoutMs: result.data.HYPERSHELL_API_TIMEOUT_MS,
    buildVersion: result.data.HYPERSHELL_BUILD_VERSION,
    host: result.data.HOST,
    logLevel: result.data.LOG_LEVEL,
    nodeEnv: result.data.NODE_ENV,
    oidcClientId: result.data.OIDC_CLIENT_ID,
    oidcIssuer: result.data.OIDC_ISSUER,
    oidcPostLogoutRedirectUri: result.data.OIDC_POST_LOGOUT_REDIRECT_URI,
    oidcRedirectUri: result.data.OIDC_REDIRECT_URI,
    port: result.data.PORT,
    sessionSecret: result.data.SESSION_SECRET
      ? Buffer.from(result.data.SESSION_SECRET, "hex")
      : undefined,
    sessionTtlSeconds: result.data.SESSION_TTL_SECONDS,
    staticRoot: path.resolve(result.data.STATIC_ROOT),
    tracing: result.data.OTEL_EXPORTER_OTLP_ENDPOINT
      ? {
          collectorEndpoint: result.data.OTEL_EXPORTER_OTLP_ENDPOINT,
          sampleRatio: result.data.OTEL_TRACES_SAMPLE_RATIO,
          serviceName: result.data.OTEL_SERVICE_NAME,
          tracesEndpoint: tracesEndpointFor(
            result.data.OTEL_EXPORTER_OTLP_ENDPOINT,
          ),
        }
      : undefined,
  };
}
