import secureSession from "@fastify/secure-session";
import type { FastifyInstance } from "fastify";
import * as oidc from "openid-client";

import type { ServerConfig } from "./config.js";
import {
  createRefresher,
  sanitizeReturnTo,
  toTokenSet,
  type Refresher,
  type TokenSet,
} from "./tokens.js";

// Fields held in the hot-path identity cookie (`session`), read on every
// proxied `/api/*` request, plus the short-lived login-flow fields.
declare module "@fastify/secure-session" {
  interface SessionData {
    accessToken?: string;
    email?: string;
    expiresAt?: number;
    name?: string;
    nonce?: string;
    pkceVerifier?: string;
    preferredUsername?: string;
    returnTo?: string;
    roles?: string[];
    state?: string;
    sub?: string;
  }
}

// Bulkier tokens live in a second cookie (`session_tok`) read only on refresh
// and logout, so no single cookie approaches the browser per-cookie size limit.
interface TokenSessionData {
  idToken?: string;
  refreshToken?: string;
}

declare module "fastify" {
  interface FastifyInstance {
    // Exchanges a refresh token for a fresh token set. Present only when OIDC
    // is configured; the `/api/*` proxy guards on `config.oidcIssuer`.
    refreshAccessToken?: Refresher;
  }
  interface FastifyRequest {
    tokenSession: secureSession.Session<TokenSessionData>;
  }
}

/** Persists a refreshed token set across the two session cookies. */
export function persistTokenSet(
  request: {
    session: secureSession.Session;
    tokenSession: secureSession.Session<TokenSessionData>;
  },
  tokens: TokenSet,
): void {
  request.session.set("accessToken", tokens.accessToken);
  request.session.set("expiresAt", tokens.expiresAt);
  if (tokens.refreshToken !== undefined) {
    request.tokenSession.set("refreshToken", tokens.refreshToken);
  }
  if (tokens.idToken !== undefined) {
    request.tokenSession.set("idToken", tokens.idToken);
  }
}

/** Clears both session cookies on terminal authentication failure. */
export function clearSession(request: {
  session: secureSession.Session;
  tokenSession: secureSession.Session<TokenSessionData>;
}): void {
  request.session.delete();
  request.tokenSession.delete();
}

/**
 * Registers OIDC-based authentication on the Fastify instance.
 *
 * This sets up encrypted cookie sessions via @fastify/secure-session, performs
 * OpenID Connect discovery against the configured issuer, and mounts the four
 * auth endpoints (/auth/login, /auth/callback, /auth/logout, /auth/session).
 *
 * Call this function only when OIDC configuration is present. It must be called
 * before route registration so that the session decorator is available to all
 * subsequent handlers.
 */
export async function registerAuth(
  app: FastifyInstance,
  config: ServerConfig,
): Promise<void> {
  if (!config.oidcIssuer || !config.oidcClientId || !config.sessionSecret) {
    return;
  }

  // --- OIDC discovery ---

  const issuerUrl = new URL(config.oidcIssuer);
  const execute: ((c: oidc.Configuration) => void)[] = [];
  if (issuerUrl.protocol === "http:") {
    // eslint-disable-next-line @typescript-eslint/no-deprecated
    execute.push(oidc.allowInsecureRequests);
  }

  const oidcConfig = await oidc.discovery(
    issuerUrl,
    config.oidcClientId,
    undefined,
    oidc.None(),
    execute.length > 0 ? { execute } : undefined,
  );

  // --- Encrypted cookie sessions (chunked to stay under the browser limit) ---

  const cookie = {
    httpOnly: true,
    path: "/",
    sameSite: "lax" as const,
    secure: true,
  };

  await app.register(secureSession, [
    {
      key: config.sessionSecret,
      cookie,
      cookieName: "session",
      expiry: config.sessionTtlSeconds,
      sessionName: "session",
    },
    {
      key: config.sessionSecret,
      cookie,
      cookieName: "session_tok",
      expiry: config.sessionTtlSeconds,
      sessionName: "tokenSession",
    },
  ]);

  // Single-flight refresher shared by the API proxy for silent token refresh.
  app.decorate("refreshAccessToken", createRefresher(oidcConfig));

  const configuredRedirectUri = config.oidcRedirectUri;

  // --- Auth routes ---

  app.get("/auth/login", async (request, reply) => {
    const codeVerifier = oidc.randomPKCECodeVerifier();
    const codeChallenge = await oidc.calculatePKCECodeChallenge(codeVerifier);
    const state = oidc.randomState();
    const nonce = oidc.randomNonce();
    const returnTo = sanitizeReturnTo(
      (request.query as { return_to?: unknown }).return_to,
    );

    request.session.set("pkceVerifier", codeVerifier);
    request.session.set("state", state);
    request.session.set("nonce", nonce);
    if (returnTo !== undefined) {
      request.session.set("returnTo", returnTo);
    }
    request.session.options({ maxAge: 300 });

    const effectiveRedirectUri =
      configuredRedirectUri ??
      `${request.protocol}://${request.hostname}/auth/callback`;

    const authUrl = oidc.buildAuthorizationUrl(oidcConfig, {
      code_challenge: codeChallenge,
      code_challenge_method: "S256",
      nonce,
      redirect_uri: effectiveRedirectUri,
      scope: "openid profile email",
      state,
    });

    reply.redirect(authUrl.toString());
  });

  app.get("/auth/callback", async (request, reply) => {
    const storedState = request.session.get("state");
    const storedNonce = request.session.get("nonce");
    const storedVerifier = request.session.get("pkceVerifier");
    const storedReturnTo = sanitizeReturnTo(request.session.get("returnTo"));

    if (!storedState || !storedNonce || !storedVerifier) {
      reply.code(400);
      return { error: "Missing or expired login session", statusCode: 400 };
    }

    const effectiveRedirectUri =
      configuredRedirectUri ??
      `${request.protocol}://${request.hostname}/auth/callback`;
    const callbackOrigin = new URL(effectiveRedirectUri).origin;
    const callbackUrl = new URL(request.url, callbackOrigin);

    try {
      const tokens = await oidc.authorizationCodeGrant(
        oidcConfig,
        callbackUrl,
        {
          expectedNonce: storedNonce,
          expectedState: storedState,
          pkceCodeVerifier: storedVerifier,
        },
      );

      const claims = tokens.claims();

      // Replace login session data with auth session data. Rotate both cookies
      // so no pre-login value survives (session fixation defense).
      request.session.regenerate();
      request.tokenSession.regenerate();

      persistTokenSet(request, toTokenSet(tokens));
      if (claims) {
        request.session.set("sub", claims.sub);
        if (typeof claims.preferred_username === "string") {
          request.session.set("preferredUsername", claims.preferred_username);
        }
        if (typeof claims.email === "string") {
          request.session.set("email", claims.email);
        }
        if (typeof claims.name === "string") {
          request.session.set("name", claims.name);
        }
        const rawRoles = claims.roles;
        const roles = Array.isArray(rawRoles)
          ? rawRoles.filter((r): r is string => typeof r === "string")
          : [];
        request.session.set("roles", roles);
      }

      request.session.options({ maxAge: config.sessionTtlSeconds });
      request.tokenSession.options({ maxAge: config.sessionTtlSeconds });

      reply.redirect(storedReturnTo ?? "/");
    } catch (error) {
      request.log.error({ err: error }, "OIDC callback failed");
      clearSession(request);
      reply.code(401);
      return { error: "Authentication failed", statusCode: 401 };
    }
  });

  app.get("/auth/logout", async (request, reply) => {
    const idToken = request.tokenSession.get("idToken");
    clearSession(request);

    const serverMetadata = oidcConfig.serverMetadata();
    if (serverMetadata.end_session_endpoint) {
      const params: Record<string, string> = {};
      if (idToken) {
        params.id_token_hint = idToken;
      }
      if (config.oidcPostLogoutRedirectUri) {
        params.post_logout_redirect_uri = config.oidcPostLogoutRedirectUri;
      }
      const logoutUrl = oidc.buildEndSessionUrl(oidcConfig, params);
      reply.redirect(logoutUrl.toString());
      return;
    }

    reply.redirect("/");
  });

  app.get("/auth/session", async (request, reply) => {
    reply.header("Cache-Control", "no-store");

    const accessToken = request.session.get("accessToken");
    if (!accessToken) {
      return { authenticated: false };
    }

    // An expired access token is not terminal: the proxy refreshes it silently
    // on the next `/api/*` call, and an unrefreshable session is surfaced there
    // as a re-authentication signal. The session resource still reports
    // `expires_at` so the browser can display re-authentication state.
    const expiresAt = request.session.get("expiresAt");
    const roles = request.session.get("roles") ?? [];

    return {
      authenticated: true,
      expires_at: expiresAt,
      roles,
      user: {
        email: request.session.get("email"),
        name: request.session.get("name"),
        preferred_username: request.session.get("preferredUsername"),
        sub: request.session.get("sub"),
      },
    };
  });
}
