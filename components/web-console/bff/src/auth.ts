import secureSession from "@fastify/secure-session";
import type { FastifyInstance } from "fastify";
import * as oidc from "openid-client";

import type { ServerConfig } from "./config.js";
import { evaluateGithubOrgGate } from "./github-org-gate.js";
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

function normalizeRoleName(role: string): string {
  return role.startsWith("/") ? role.slice(1) : role;
}

function readStringClaim(
  claims: Record<string, unknown>,
  claimName: string,
): string[] | undefined {
  const rawRoles = claims[claimName];
  if (!Array.isArray(rawRoles)) {
    return undefined;
  }
  return rawRoles
    .filter((role): role is string => typeof role === "string")
    .map(normalizeRoleName);
}

function readRealmAccessRoles(claims: Record<string, unknown>): string[] {
  const realmAccess = claims.realm_access;
  if (
    typeof realmAccess !== "object" ||
    realmAccess === null ||
    Array.isArray(realmAccess)
  ) {
    return [];
  }
  const rawRoles = (realmAccess as Record<string, unknown>).roles;
  if (!Array.isArray(rawRoles)) {
    return [];
  }
  return rawRoles
    .filter((role): role is string => typeof role === "string")
    .map(normalizeRoleName);
}

/**
 * Reads realm roles from OIDC claims emitted by HyperShell Keycloak.
 *
 * The hypershell-frontend client maps realm roles into the top-level `groups`
 * claim on ID tokens via `oidc-usermodel-realm-role-mapper` (not Keycloak group
 * paths). Access tokens may also carry `realm_access.roles`. The `roles` claim
 * is a BFF/session convention when present. Group-path values such as
 * `/hypershell-admins` are normalized by stripping a leading slash.
 */
export function extractRealmRoles(claims: Record<string, unknown>): string[] {
  const roles = readStringClaim(claims, "roles");
  if (roles !== undefined) {
    return roles;
  }

  const groups = readStringClaim(claims, "groups");
  if (groups !== undefined) {
    return groups;
  }

  return readRealmAccessRoles(claims);
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
 * Standalone HTML for `/auth/denied`. Helmet's global CSP is `style-src
 * 'self'`, so the inline stylesheet is admitted only via a sha256 hash of
 * this document's `<style>` body (see `inlineStyleHashes` in app.ts).
 */
export const AUTH_DENIED_PAGE_HTML = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8"/>
    <meta name="viewport" content="width=device-width, initial-scale=1"/>
    <title>Access denied</title>
    <style>
      :root {
        color-scheme: light dark;
        --hs-bg: #f2f2f2;
        --hs-card-bg: #ffffff;
        --hs-border: #e0e0e0;
        --hs-text: #151515;
        --hs-text-secondary: #4d4d4d;
        --hs-accent: #0066cc;
        --hs-accent-hover: #004d99;
      }
      @media (prefers-color-scheme: dark) {
        :root {
          --hs-bg: #1b1d21;
          --hs-card-bg: #292e34;
          --hs-border: #3c3f42;
          --hs-text: #e0e0e0;
          --hs-text-secondary: #a2a2a2;
          --hs-accent: #73bcf7;
          --hs-accent-hover: #bee1f4;
        }
      }
      * {
        box-sizing: border-box;
      }
      body {
        margin: 0;
        min-height: 100vh;
        display: flex;
        align-items: center;
        justify-content: center;
        background: var(--hs-bg);
        color: var(--hs-text);
        font-family: "Red Hat Text", system-ui, -apple-system, sans-serif;
      }
      .card {
        width: 100%;
        max-width: 30rem;
        margin: 1.5rem;
        padding: 2.5rem 2rem;
        background: var(--hs-card-bg);
        border: 1px solid var(--hs-border);
        border-radius: 0.625rem;
        box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
        text-align: center;
      }
      .brand {
        font-size: 1.75rem;
        font-weight: 600;
        margin: 0 0 1.5rem;
      }
      h1 {
        font-size: 1.25rem;
        margin: 0 0 1rem;
      }
      p {
        color: var(--hs-text-secondary);
        line-height: 1.5;
        margin: 0 0 1rem;
      }
      a {
        color: var(--hs-accent);
        font-weight: 600;
        text-decoration: none;
      }
      a:hover {
        color: var(--hs-accent-hover);
        text-decoration: underline;
      }
    </style>
  </head>
  <body>
    <main class="card">
      <p class="brand">HyperShell</p>
      <h1>Access denied</h1>
      <p>This HyperShell environment is limited to members of the configured GitHub organization and allowlisted usernames.</p>
      <p><a href="/auth/logout">Sign out</a> and try a different GitHub account or contact the maintainers of this project to be added to the allowlist.</p>
    </main>
  </body>
</html>
`;

/**
 * Registers OIDC-based authentication on the Fastify instance.
 *
 * This sets up encrypted cookie sessions via @fastify/secure-session, performs
 * OpenID Connect discovery against the configured issuer, and mounts the
 * auth endpoints (/auth/login, /auth/callback, /auth/denied, /auth/logout,
 * /auth/session).
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

  app.get("/auth/denied", async (_request, reply) => {
    reply
      .code(403)
      .header("Cache-Control", "no-store")
      .type("text/html; charset=utf-8");
    return AUTH_DENIED_PAGE_HTML;
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
      const tokenSet = toTokenSet(tokens);

      // Pull-request environments set GITHUB_ORG_GATE so interactive GitHub
      // logins are limited to org members and allowlisted usernames. Kind and
      // local leave it unset, so password users are not checked.
      if (config.githubOrgGate && config.oidcIssuer) {
        const username =
          typeof claims?.preferred_username === "string"
            ? claims.preferred_username
            : undefined;
        const allowed = await evaluateGithubOrgGate({
          accessToken: tokenSet.accessToken,
          allowlistRaw: config.githubUsernameAllowlist,
          githubApiOrigin: config.githubApiOrigin ?? "https://api.github.com",
          oidcIssuer: config.oidcIssuer,
          onLookupError: (error) => {
            request.log.warn(
              { err: error, preferredUsername: username },
              "GitHub org gate lookup failed",
            );
          },
          orgGate: config.githubOrgGate,
          username,
        });
        if (!allowed) {
          request.log.info(
            { preferredUsername: username },
            "GitHub org gate denied login",
          );
          clearSession(request);
          reply.redirect("/auth/denied");
          return;
        }
      }

      // Replace login session data with auth session data. Rotate both cookies
      // so no pre-login value survives (session fixation defense).
      request.session.regenerate();
      request.tokenSession.regenerate();

      persistTokenSet(request, tokenSet);
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
        request.session.set("roles", extractRealmRoles(claims));
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
