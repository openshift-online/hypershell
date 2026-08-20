import { reactRouter } from "@react-router/dev/vite";
import { defineConfig, type Plugin } from "vite";

const getApiOrigin = (): string => {
  const configuredOrigin =
    process.env.WEB_CONSOLE_API_ORIGIN ?? "http://127.0.0.1:8000";
  const origin = new URL(configuredOrigin);
  const isLoopback = ["127.0.0.1", "::1", "localhost"].includes(
    origin.hostname,
  );

  if (
    !["http:", "https:"].includes(origin.protocol) ||
    origin.username ||
    origin.password
  ) {
    throw new Error(
      "WEB_CONSOLE_API_ORIGIN must be an HTTP(S) origin without credentials",
    );
  }
  if (!isLoopback) {
    throw new Error(
      "WEB_CONSOLE_API_ORIGIN must use a loopback host for no-auth development",
    );
  }

  return origin.origin;
};

interface DevAuthConfig {
  clientId: string;
  password: string;
  tokenUrl: string;
  username: string;
}

// Trimmed environment value, or the fallback when unset or blank.
const envOrDefault = (value: string | undefined, fallback: string): string => {
  const trimmed = value?.trim() ?? "";
  return trimmed.length > 0 ? trimmed : fallback;
};

// In production the BFF injects the OIDC bearer token onto /api requests. The
// hot-reload dev server (`react-router dev`) runs Vite without the BFF, so it
// must mint and inject a token itself when pointed at an OIDC-enforcing API
// server (see scripts/kind/swap-component.sh). Returns undefined for no-auth
// development, in which case /api is proxied without an Authorization header.
const getDevAuthConfig = (): DevAuthConfig | undefined => {
  const issuer = process.env.OIDC_ISSUER?.trim();
  const clientId = process.env.OIDC_CLIENT_ID?.trim();
  if (!issuer || !clientId) {
    return undefined;
  }
  return {
    clientId,
    password: envOrDefault(process.env.KIND_DEV_PASSWORD, "admin"),
    tokenUrl: `${issuer.replace(/\/+$/u, "")}/protocol/openid-connect/token`,
    username: envOrDefault(process.env.KIND_DEV_USER, "admin"),
  };
};

interface CachedToken {
  expiresAtMs: number;
  value: string;
}

// Returns an access-token getter that mints a token via the Keycloak resource
// owner password grant, caches it, and re-mints shortly before expiry.
// Concurrent callers share a single in-flight mint.
const createTokenProvider = (
  config: DevAuthConfig,
): (() => Promise<string>) => {
  const refreshSkewMs = 30_000;
  let cached: CachedToken | undefined;
  let pending: Promise<CachedToken> | undefined;

  const mint = async (): Promise<CachedToken> => {
    const response = await fetch(config.tokenUrl, {
      body: new URLSearchParams({
        client_id: config.clientId,
        grant_type: "password",
        password: config.password,
        scope: "openid",
        username: config.username,
      }),
      headers: { "content-type": "application/x-www-form-urlencoded" },
      method: "POST",
    });
    if (!response.ok) {
      const detail = await response.text().catch(() => "");
      throw new Error(
        `Keycloak token request failed (${String(response.status)}): ${detail}`,
      );
    }
    const payload = (await response.json()) as unknown;
    if (
      typeof payload !== "object" ||
      payload === null ||
      typeof (payload as { access_token?: unknown }).access_token !== "string"
    ) {
      throw new Error("Keycloak token response missing access_token");
    }
    const expiresIn = (payload as { expires_in?: unknown }).expires_in;
    return {
      expiresAtMs:
        Date.now() + (typeof expiresIn === "number" ? expiresIn : 60) * 1000,
      value: (payload as { access_token: string }).access_token,
    };
  };

  return async (): Promise<string> => {
    if (cached && cached.expiresAtMs - refreshSkewMs > Date.now()) {
      return cached.value;
    }
    pending ??= mint().finally(() => {
      pending = undefined;
    });
    cached = await pending;
    return cached.value;
  };
};

// Dev-only: mint a Keycloak token and attach it to proxied /api requests so the
// hot-reload console can talk to an OIDC-enforcing API server. Runs before
// Vite's built-in proxy so the mutated Authorization header is forwarded.
const devAuthProxyPlugin = (): Plugin => {
  const config = getDevAuthConfig();
  return {
    apply: "serve",
    configureServer(server) {
      if (!config) {
        server.config.logger.info(
          "[dev-auth] OIDC_ISSUER/OIDC_CLIENT_ID unset; proxying /api without a bearer token (no-auth dev).",
        );
        return;
      }
      // Keycloak in the Kind dev cluster serves a self-signed certificate. The
      // swap script exports NODE_TLS_REJECT_UNAUTHORIZED=0; set it here too so a
      // standalone `pnpm dev` against the dev cluster works. Dev server only.
      process.env.NODE_TLS_REJECT_UNAUTHORIZED ??= "0";
      const getToken = createTokenProvider(config);
      server.config.logger.info(
        `[dev-auth] Injecting Keycloak bearer token on /api as "${config.username}" (set KIND_DEV_USER/KIND_DEV_PASSWORD to change).`,
      );
      server.middlewares.use((req, res, next) => {
        if (!req.url?.startsWith("/api")) {
          next();
          return;
        }
        getToken()
          .then((token) => {
            req.headers.authorization = `Bearer ${token}`;
            next();
          })
          .catch((error: unknown) => {
            server.config.logger.error(
              `[dev-auth] Failed to mint Keycloak token: ${
                error instanceof Error ? error.message : "unknown error"
              }`,
            );
            next();
          });
      });
    },
    name: "hypershell-dev-auth-proxy",
  };
};

export default defineConfig({
  envDir: false,
  plugins:
    process.env.STORYBOOK === "true"
      ? []
      : [reactRouter(), devAuthProxyPlugin()],
  build: {
    sourcemap: false,
    target: "es2022",
  },
  server: {
    host: process.env.DEV_SERVER_HOST ?? "127.0.0.1",
    port: 5173,
    strictPort: true,
    proxy: {
      "/api": {
        target: getApiOrigin(),
        changeOrigin: false,
      },
    },
  },
  ssr: {
    noExternal: [/^@patternfly\//],
  },
});
