import { createPublicKey, generateKeyPairSync, sign } from "node:crypto";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import {
  createServer,
  type IncomingMessage,
  type Server,
  type ServerResponse,
} from "node:http";
import { tmpdir } from "node:os";
import path from "node:path";

import type { FastifyInstance } from "fastify";

import { buildApp } from "../src/app.js";
import { loadConfig, type ServerConfig } from "../src/config.js";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const { privateKey } = generateKeyPairSync("rsa", { modulusLength: 2048 });
const jwk = createPublicKey(privateKey).export({ format: "jwk" });

function createJwt(claims: Record<string, unknown>): string {
  const header = { alg: "RS256", kid: "test-key-1", typ: "JWT" };
  const h = Buffer.from(JSON.stringify(header)).toString("base64url");
  const p = Buffer.from(JSON.stringify(claims)).toString("base64url");
  const sig = sign("sha256", Buffer.from(`${h}.${p}`), privateKey).toString(
    "base64url",
  );
  return `${h}.${p}.${sig}`;
}

function sessionCookie(response: { headers: Record<string, unknown> }): string {
  const raw = response.headers["set-cookie"];
  const entries: unknown[] = Array.isArray(raw) ? raw : [raw];
  for (const entry of entries) {
    if (typeof entry !== "string") continue;
    if (entry.startsWith("session=")) {
      return entry.split(";")[0] ?? "";
    }
  }
  return "";
}

const testSessionSecret =
  "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef";

// ---------------------------------------------------------------------------
// Mock OIDC provider
// ---------------------------------------------------------------------------

interface OidcContext {
  nonce: string;
  port: number;
}

function createOidcServer(ctx: OidcContext): Server {
  return createServer((req: IncomingMessage, res: ServerResponse) => {
    const origin = `http://127.0.0.1:${String(ctx.port)}`;
    const url = new URL(req.url ?? "/", origin);

    if (url.pathname === "/.well-known/openid-configuration") {
      res.setHeader("content-type", "application/json");
      res.end(
        JSON.stringify({
          authorization_endpoint: `${origin}/authorize`,
          code_challenge_methods_supported: ["S256"],
          end_session_endpoint: `${origin}/end-session`,
          id_token_signing_alg_values_supported: ["RS256"],
          issuer: origin,
          jwks_uri: `${origin}/jwks`,
          response_types_supported: ["code"],
          subject_types_supported: ["public"],
          token_endpoint: `${origin}/token`,
        }),
      );
      return;
    }

    if (url.pathname === "/jwks") {
      res.setHeader("content-type", "application/json");
      res.end(
        JSON.stringify({ keys: [{ ...jwk, kid: "test-key-1", use: "sig" }] }),
      );
      return;
    }

    if (url.pathname === "/token" && req.method === "POST") {
      const chunks: Buffer[] = [];
      req.on("data", (chunk: Buffer) => chunks.push(chunk));
      req.on("end", () => {
        const idToken = createJwt({
          aud: "test-client",
          email: "test@example.com",
          exp: Math.floor(Date.now() / 1000) + 3600,
          iat: Math.floor(Date.now() / 1000),
          iss: origin,
          name: "Test User",
          nonce: ctx.nonce,
          preferred_username: "testuser",
          roles: ["admin", "viewer"],
          sub: "user-123",
        });

        res.setHeader("content-type", "application/json");
        res.end(
          JSON.stringify({
            access_token: "test-access-token",
            expires_in: 3600,
            id_token: idToken,
            token_type: "Bearer",
          }),
        );
      });
      return;
    }

    res.statusCode = 404;
    res.end();
  });
}

// ---------------------------------------------------------------------------
// Configuration validation
// ---------------------------------------------------------------------------

describe("OIDC configuration validation", () => {
  it("rejects partial OIDC config when OIDC_CLIENT_ID is missing", () => {
    expect(() =>
      loadConfig({
        OIDC_ISSUER: "https://idp.example.com",
        SESSION_SECRET: testSessionSecret,
      }),
    ).toThrow(/OIDC_CLIENT_ID is required when OIDC_ISSUER is set/u);
  });

  it("rejects partial OIDC config when SESSION_SECRET is missing", () => {
    expect(() =>
      loadConfig({
        OIDC_CLIENT_ID: "my-client",
        OIDC_ISSUER: "https://idp.example.com",
      }),
    ).toThrow(/SESSION_SECRET is required when OIDC_ISSUER is set/u);
  });

  it("reports both missing fields when OIDC is partially configured", () => {
    expect(() =>
      loadConfig({ OIDC_ISSUER: "https://idp.example.com" }),
    ).toThrow(/OIDC_CLIENT_ID.*SESSION_SECRET|SESSION_SECRET.*OIDC_CLIENT_ID/u);
  });

  it("accepts a complete OIDC configuration", () => {
    const config = loadConfig({
      OIDC_CLIENT_ID: "my-client",
      OIDC_ISSUER: "https://idp.example.com",
      SESSION_SECRET: testSessionSecret,
    });
    expect(config.oidcIssuer).toBe("https://idp.example.com");
    expect(config.oidcClientId).toBe("my-client");
    expect(config.sessionSecret).toBeInstanceOf(Buffer);
    expect(config.sessionSecret?.length).toBe(32);
    expect(config.sessionTtlSeconds).toBe(28_800);
  });

  it("accepts configuration without any OIDC settings", () => {
    const config = loadConfig({});
    expect(config.oidcIssuer).toBeUndefined();
    expect(config.sessionSecret).toBeUndefined();
  });
});

// ---------------------------------------------------------------------------
// Auth behaviour (OIDC enabled)
// ---------------------------------------------------------------------------

describe("web-console BFF with OIDC enabled", () => {
  let app: FastifyInstance;
  let apiServer: Server;
  let oidcServer: Server;
  let staticRoot: string;
  let apiRequests: {
    body: string;
    headers: Record<string, string | string[] | undefined>;
    method: string | undefined;
    url: string | undefined;
  }[];
  const oidcCtx: OidcContext = { nonce: "", port: 0 };

  beforeEach(async () => {
    apiRequests = [];

    // --- mock upstream API ---
    apiServer = createServer(
      (request: IncomingMessage, response: ServerResponse) => {
        const chunks: Buffer[] = [];
        request.on("data", (chunk: Buffer) => chunks.push(chunk));
        request.on("end", () => {
          apiRequests.push({
            body: Buffer.concat(chunks).toString("utf8"),
            headers: request.headers,
            method: request.method,
            url: request.url,
          });
          response.setHeader("content-type", "application/json; charset=utf-8");
          response.statusCode = 200;
          response.end('{"kind":"GatewayList","items":[]}');
        });
      },
    );
    await new Promise<void>((r) => {
      apiServer.listen(0, "127.0.0.1", r);
    });
    const apiAddr = apiServer.address();
    if (apiAddr === null || typeof apiAddr === "string") {
      throw new Error("Expected TCP address for API server");
    }

    // --- mock OIDC provider (must start before buildApp) ---
    oidcServer = createOidcServer(oidcCtx);
    await new Promise<void>((r) => {
      oidcServer.listen(0, "127.0.0.1", r);
    });
    const oidcAddr = oidcServer.address();
    if (oidcAddr === null || typeof oidcAddr === "string") {
      throw new Error("Expected TCP address for OIDC server");
    }
    oidcCtx.port = oidcAddr.port;

    // --- static assets ---
    staticRoot = await mkdtemp(path.join(tmpdir(), "hypershell-web-console-"));
    await mkdir(path.join(staticRoot, "assets"));
    await writeFile(
      path.join(staticRoot, "index.html"),
      "<!doctype html><html><body><script>globalThis.ready = true;</script><main>App</main></body></html>",
    );
    await writeFile(
      path.join(staticRoot, "assets", "app-deadbeef.js"),
      'console.log("asset");',
    );

    // --- build app ---
    const config: ServerConfig = {
      apiOrigin: `http://127.0.0.1:${String(apiAddr.port)}`,
      apiTimeoutMs: 5000,
      host: "127.0.0.1",
      logLevel: "silent",
      nodeEnv: "test",
      oidcClientId: "test-client",
      oidcIssuer: `http://127.0.0.1:${String(oidcCtx.port)}`,
      oidcRedirectUri: `http://127.0.0.1:8080/auth/callback`,
      port: 8080,
      sessionSecret: Buffer.from(testSessionSecret, "hex"),
      sessionTtlSeconds: 28_800,
      staticRoot,
    };
    app = await buildApp(config);
  });

  afterEach(async () => {
    await app.close();
    await new Promise<void>((resolve, reject) => {
      apiServer.close((e) => {
        if (e) {
          reject(e);
        } else {
          resolve();
        }
      });
    });
    await new Promise<void>((resolve, reject) => {
      oidcServer.close((e) => {
        if (e) {
          reject(e);
        } else {
          resolve();
        }
      });
    });
    await rm(staticRoot, { force: true, recursive: true });
  });

  // --- helper: create an auth session cookie with test data ---
  function authenticateSession(): string {
    const session = app.createSecureSession({
      accessToken: "test-access-token",
      email: "test@example.com",
      expiresAt: Math.floor(Date.now() / 1000) + 3600,
      idToken: "fake-id-token",
      name: "Test User",
      preferredUsername: "testuser",
      roles: ["admin", "viewer"],
      sub: "user-123",
    });
    // URL-encode the value because @fastify/cookie URL-decodes the Cookie header
    return `session=${encodeURIComponent(app.encodeSecureSession(session))}`;
  }

  // -----------------------------------------------------------------------
  // Auth endpoints
  // -----------------------------------------------------------------------

  it("redirects /auth/login to the IdP authorization endpoint with PKCE", async () => {
    const response = await app.inject({ method: "GET", url: "/auth/login" });

    expect(response.statusCode).toBe(302);
    const locationHeader = response.headers.location;
    if (typeof locationHeader !== "string") {
      throw new Error("Expected location header to be a string");
    }
    const location = new URL(locationHeader);
    expect(location.pathname).toBe("/authorize");
    expect(location.searchParams.get("client_id")).toBe("test-client");
    expect(location.searchParams.get("response_type")).toBe("code");
    expect(location.searchParams.get("scope")).toBe("openid profile email");
    expect(location.searchParams.get("code_challenge_method")).toBe("S256");
    expect(location.searchParams.get("code_challenge")).toBeTruthy();
    expect(location.searchParams.get("state")).toBeTruthy();
    expect(location.searchParams.get("nonce")).toBeTruthy();
    expect(location.searchParams.get("redirect_uri")).toBe(
      "http://127.0.0.1:8080/auth/callback",
    );

    // A session cookie must be set for the callback to verify state/nonce
    expect(sessionCookie(response)).toContain("session=");
  });

  it("exchanges code for tokens on /auth/callback and redirects to /", async () => {
    // Step 1: start the login to capture state, nonce, and login cookie
    const login = await app.inject({ method: "GET", url: "/auth/login" });
    if (typeof login.headers.location !== "string") {
      throw new Error("Expected location header");
    }
    const redirectUrl = new URL(login.headers.location);
    const state = redirectUrl.searchParams.get("state");
    const nonce = redirectUrl.searchParams.get("nonce");
    if (!state || !nonce) {
      throw new Error("Expected state and nonce in redirect URL");
    }

    // Tell the mock token endpoint which nonce to embed in the id_token
    oidcCtx.nonce = nonce;

    // Step 2: simulate the IdP redirect back to /auth/callback
    const callback = await app.inject({
      headers: { cookie: sessionCookie(login) },
      method: "GET",
      url: `/auth/callback?code=test-code&state=${state}`,
    });

    expect(callback.statusCode).toBe(302);
    expect(callback.headers.location).toBe("/");

    // Step 3: verify the auth session contains user data
    const session = await app.inject({
      headers: { cookie: sessionCookie(callback) },
      method: "GET",
      url: "/auth/session",
    });
    const body: Record<string, unknown> = session.json();
    expect(body.authenticated).toBe(true);
    expect(body).toHaveProperty("user");
    const user = body.user as Record<string, unknown>;
    expect(user.sub).toBe("user-123");
    expect(user.preferred_username).toBe("testuser");
    expect(user.email).toBe("test@example.com");
    expect(user.name).toBe("Test User");
    expect(body.roles).toEqual(["admin", "viewer"]);
    expect(body.expires_at).toEqual(expect.any(Number));
  });

  it("returns 400 on /auth/callback without a prior login session", async () => {
    const response = await app.inject({
      method: "GET",
      url: "/auth/callback?code=abc&state=xyz",
    });

    expect(response.statusCode).toBe(400);
    expect(response.json()).toMatchObject({ statusCode: 400 });
  });

  it("returns authenticated: false from /auth/session without a cookie", async () => {
    const response = await app.inject({
      method: "GET",
      url: "/auth/session",
    });

    expect(response.statusCode).toBe(200);
    expect(response.json()).toEqual({ authenticated: false });
    expect(response.headers["cache-control"]).toBe("no-store");
  });

  it("returns session data from /auth/session with a valid cookie", async () => {
    const cookie = authenticateSession();
    const response = await app.inject({
      headers: { cookie },
      method: "GET",
      url: "/auth/session",
    });

    const body: Record<string, unknown> = response.json();
    expect(body.authenticated).toBe(true);
    expect((body.user as Record<string, unknown>).sub).toBe("user-123");
    expect(body.roles).toEqual(["admin", "viewer"]);
  });

  it("clears session and redirects on /auth/logout", async () => {
    const cookie = authenticateSession();
    const response = await app.inject({
      headers: { cookie },
      method: "GET",
      url: "/auth/logout",
    });

    expect(response.statusCode).toBe(302);
    // Should redirect to the IdP end_session_endpoint
    if (typeof response.headers.location !== "string") {
      throw new Error("Expected location header");
    }
    const location = new URL(response.headers.location);
    expect(location.pathname).toBe("/end-session");

    // Session should be cleared: /auth/session returns unauthenticated
    const session = await app.inject({
      headers: { cookie: sessionCookie(response) },
      method: "GET",
      url: "/auth/session",
    });
    expect(session.json()).toEqual({ authenticated: false });
  });

  it("redirects /auth/logout to IdP end_session_endpoint without id_token_hint", async () => {
    const session = app.createSecureSession({
      accessToken: "test-access-token",
    });
    const cookie = `session=${encodeURIComponent(app.encodeSecureSession(session))}`;

    const response = await app.inject({
      headers: { cookie },
      method: "GET",
      url: "/auth/logout",
    });

    expect(response.statusCode).toBe(302);
    expect(response.headers.location).toBeDefined();
    expect(response.headers.location).toContain("/end-session");
    expect(response.headers.location).not.toContain("id_token_hint");
  });

  // -----------------------------------------------------------------------
  // CSRF protection
  // -----------------------------------------------------------------------

  it("rejects cross-origin POST requests with 403", async () => {
    const cookie = authenticateSession();
    const response = await app.inject({
      headers: {
        cookie,
        "content-type": "application/json",
        host: "app.example.com",
        origin: "https://evil.example.com",
      },
      method: "POST",
      payload: { name: "test" },
      url: "/api/hypershell/v1/gateways",
    });

    expect(response.statusCode).toBe(403);
    expect(response.json()).toMatchObject({ statusCode: 403 });
    expect(apiRequests).toHaveLength(0);
  });

  it("allows same-origin POST requests", async () => {
    const cookie = authenticateSession();
    const response = await app.inject({
      headers: {
        cookie,
        "content-type": "application/json",
        host: "localhost",
        origin: "http://localhost",
      },
      method: "POST",
      payload: { name: "test" },
      url: "/api/hypershell/v1/gateways",
    });

    expect(response.statusCode).not.toBe(403);
    expect(apiRequests).toHaveLength(1);
  });

  it("allows mutating requests without an Origin header", async () => {
    const cookie = authenticateSession();
    const response = await app.inject({
      headers: {
        cookie,
        "content-type": "application/json",
      },
      method: "POST",
      payload: { name: "test" },
      url: "/api/hypershell/v1/gateways",
    });

    expect(response.statusCode).not.toBe(403);
    expect(apiRequests).toHaveLength(1);
  });

  // -----------------------------------------------------------------------
  // Proxy auth header injection
  // -----------------------------------------------------------------------

  it("injects Bearer token into upstream API requests from session", async () => {
    const cookie = authenticateSession();
    const response = await app.inject({
      headers: { cookie },
      method: "GET",
      url: "/api/hypershell/v1/gateways",
    });

    expect(response.statusCode).toBe(200);
    expect(apiRequests).toHaveLength(1);
    expect(apiRequests[0]?.headers.authorization).toBe(
      "Bearer test-access-token",
    );
  });

  it("proxies API requests without Bearer token when unauthenticated", async () => {
    const response = await app.inject({
      method: "GET",
      url: "/api/hypershell/v1/gateways",
    });

    expect(response.statusCode).toBe(200);
    expect(apiRequests).toHaveLength(1);
    expect(apiRequests[0]?.headers.authorization).toBeUndefined();
  });

  // -----------------------------------------------------------------------
  // Auth enforcement
  // -----------------------------------------------------------------------

  it("redirects unauthenticated GETs to application routes to /auth/login", async () => {
    for (const route of ["/", "/gateways/new", "/gateways/gw-1"]) {
      const response = await app.inject({ method: "GET", url: route });
      expect(response.statusCode, route).toBe(302);
      expect(response.headers.location, route).toBe("/auth/login");
    }
  });

  it("serves application routes when authenticated", async () => {
    const cookie = authenticateSession();
    const response = await app.inject({
      headers: { cookie },
      method: "GET",
      url: "/",
    });

    expect(response.statusCode).toBe(200);
    expect(response.headers["content-type"]).toContain("text/html");
  });

  it("does not require authentication for health probes", async () => {
    const live = await app.inject({ method: "GET", url: "/health/live" });
    const ready = await app.inject({ method: "GET", url: "/health/ready" });

    expect(live.statusCode).toBe(200);
    expect(ready.statusCode).toBe(200);
  });

  it("does not require authentication for /auth/* endpoints", async () => {
    const session = await app.inject({
      method: "GET",
      url: "/auth/session",
    });

    expect(session.statusCode).toBe(200);
    expect(session.json()).toEqual({ authenticated: false });
  });

  it("does not require authentication for static assets", async () => {
    const response = await app.inject({
      method: "GET",
      url: "/assets/app-deadbeef.js",
    });

    expect(response.statusCode).toBe(200);
  });
});
