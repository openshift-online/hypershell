import secureSession from "@fastify/secure-session";
import type { FastifyInstance } from "fastify";
import * as oidc from "openid-client";

import type { ServerConfig } from "./config.js";

declare module "@fastify/secure-session" {
  interface SessionData {
    accessToken?: string;
    email?: string;
    expiresAt?: number;
    name?: string;
    nonce?: string;
    pkceVerifier?: string;
    preferredUsername?: string;
    roles?: string[];
    state?: string;
    sub?: string;
  }
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

  // --- Encrypted cookie session ---

  await app.register(secureSession, {
    key: config.sessionSecret,
    cookie: {
      httpOnly: true,
      path: "/",
      sameSite: "lax",
      secure: true,
    },
    cookieName: "session",
    expiry: config.sessionTtlSeconds,
  });

  const configuredRedirectUri = config.oidcRedirectUri;

  // --- Auth routes ---

  app.get("/auth/login", async (request, reply) => {
    const codeVerifier = oidc.randomPKCECodeVerifier();
    const codeChallenge = await oidc.calculatePKCECodeChallenge(codeVerifier);
    const state = oidc.randomState();
    const nonce = oidc.randomNonce();

    request.session.set("pkceVerifier", codeVerifier);
    request.session.set("state", state);
    request.session.set("nonce", nonce);
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

      // Replace login session data with auth session data
      request.session.regenerate();

      request.session.set("accessToken", tokens.access_token);
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

      const expiresIn = tokens.expiresIn();
      if (expiresIn !== undefined) {
        request.session.set(
          "expiresAt",
          Math.floor(Date.now() / 1000) + expiresIn,
        );
      }

      request.session.options({ maxAge: config.sessionTtlSeconds });

      reply.redirect("/");
    } catch (error) {
      request.log.error({ err: error }, "OIDC callback failed");
      request.session.delete();
      reply.code(401);
      return { error: "Authentication failed", statusCode: 401 };
    }
  });

  app.get("/auth/logout", async (request, reply) => {
    request.session.delete();

    const serverMetadata = oidcConfig.serverMetadata();
    if (serverMetadata.end_session_endpoint) {
      const params: Record<string, string> = {};
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

    const expiresAt = request.session.get("expiresAt");
    if (expiresAt !== undefined && expiresAt < Math.floor(Date.now() / 1000)) {
      request.session.delete();
      return { authenticated: false };
    }

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
