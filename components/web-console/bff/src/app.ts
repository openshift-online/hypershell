import { createHash, randomUUID } from "node:crypto";
import { readFile } from "node:fs/promises";
import path from "node:path";

import compress from "@fastify/compress";
import helmet from "@fastify/helmet";
import fastifyStatic from "@fastify/static";
import Fastify, { type FastifyInstance, LogController } from "fastify";

import { clearSession, persistTokenSet, registerAuth } from "./auth.js";
import type { ServerConfig } from "./config.js";
import { tokenExpired } from "./tokens.js";
import {
  disabledTracing,
  type BffTracing,
  type ProxyOutcome,
} from "./tracing.js";

const correlationHeader = "x-hypershell-correlation-id";
const validCorrelationId =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/iu;
const forwardedRequestHeaders = [
  "accept",
  "content-type",
  "if-match",
  "if-none-match",
] as const;
const forwardedResponseHeaders = [
  "content-type",
  "etag",
  "location",
  "retry-after",
] as const;

declare module "fastify" {
  interface FastifyRequest {
    correlationId: string;
  }
}

function isApplicationRoute(pathname: string): boolean {
  return (
    pathname === "/" ||
    pathname === "/login" ||
    pathname === "/gateways/new" ||
    /^\/gateways\/[^/]+\/?$/u.test(pathname)
  );
}

function proxyBody(
  method: string,
  body: unknown,
): string | Uint8Array | undefined {
  if (["GET", "HEAD"].includes(method) || body === undefined) {
    return undefined;
  }
  if (typeof body === "string" || body instanceof Uint8Array) {
    return body;
  }
  return JSON.stringify(body);
}

function inlineScriptHashes(document: string): string[] {
  const hashes = new Set<string>();
  const scriptPattern = /<script\b(?![^>]*\bsrc=)[^>]*>([\s\S]*?)<\/script>/giu;

  for (const match of document.matchAll(scriptPattern)) {
    const body = match[1];
    if (body) {
      hashes.add(
        `'sha256-${createHash("sha256").update(body).digest("base64")}'`,
      );
    }
  }

  return [...hashes];
}

function singleHeaderValue(
  value: string | string[] | undefined,
): string | undefined {
  return typeof value === "string" ? value : undefined;
}

function outcomeForStatus(statusCode: number): ProxyOutcome {
  if (statusCode >= 500) {
    return "server_error";
  }
  if (statusCode >= 400) {
    return "client_error";
  }
  return "success";
}

export async function buildApp(
  config: ServerConfig,
  tracing: BffTracing = disabledTracing,
): Promise<FastifyInstance> {
  const indexPath = path.join(config.staticRoot, "index.html");
  const indexDocument = await readFile(indexPath, "utf8");
  const scriptHashes = inlineScriptHashes(indexDocument);

  const app = Fastify({
    bodyLimit: 1_048_576,
    connectionTimeout: 10_000,
    keepAliveTimeout: 72_000,
    logController: new LogController({ disableRequestLogging: true }),
    logger: {
      level: config.logLevel,
      redact: {
        paths: [
          "req.headers.authorization",
          "req.headers.cookie",
          "res.headers.set-cookie",
        ],
        censor: "[REDACTED]",
      },
    },
    requestTimeout: 30_000,
    trustProxy: config.nodeEnv !== "development" || !!config.oidcIssuer,
  });

  app.decorateRequest("correlationId", "");

  app.addHook("onRequest", async (request, reply) => {
    const supplied = request.headers[correlationHeader];
    request.correlationId =
      typeof supplied === "string" && validCorrelationId.test(supplied)
        ? supplied
        : randomUUID();
    reply.header(correlationHeader, request.correlationId);
  });

  await app.register(helmet, {
    contentSecurityPolicy: {
      directives: {
        defaultSrc: ["'none'"],
        baseUri: ["'none'"],
        connectSrc: ["'self'"],
        fontSrc: ["'self'"],
        formAction: ["'self'"],
        frameAncestors: ["'none'"],
        imgSrc: ["'self'", "data:"],
        objectSrc: ["'none'"],
        scriptSrc: ["'self'", ...scriptHashes],
        styleSrc: ["'self'"],
        styleSrcAttr: ["'none'"],
        // The BFF serves plain HTTP behind the deployment TLS terminator.
        // HTTPS already blocks mixed active content, while this directive
        // breaks direct local HTTP use in WebKit by upgrading relative assets.
        upgradeInsecureRequests: null,
      },
    },
    crossOriginEmbedderPolicy: false,
    referrerPolicy: { policy: "no-referrer" },
  });

  app.addHook("onSend", async (_request, reply) => {
    reply.header(
      "Permissions-Policy",
      "camera=(), geolocation=(), microphone=(), payment=(), usb=()",
    );
  });

  app.addHook("onResponse", async (request, reply) => {
    request.log.info(
      {
        method: request.method,
        correlationId: request.correlationId,
        requestId: request.id,
        route: request.routeOptions.url,
        statusCode: reply.statusCode,
      },
      "request complete",
    );
  });

  await app.register(compress, {
    global: true,
    threshold: 1024,
  });

  if (config.oidcIssuer) {
    await registerAuth(app, config);

    // CSRF protection: reject mutating requests whose Origin does not match
    // the Host header. Requests without an Origin header are allowed through
    // (non-browser clients such as curl may omit it).
    app.addHook("onRequest", async (request, reply) => {
      if (["POST", "PATCH", "PUT", "DELETE"].includes(request.method)) {
        const origin = request.headers.origin;
        if (typeof origin === "string") {
          try {
            const originHost = new URL(origin).host;
            const requestHost = request.headers.host;
            if (!requestHost || originHost !== requestHost) {
              reply.code(403);
              reply.send({ error: "Forbidden", statusCode: 403 });
              return;
            }
          } catch {
            reply.code(403);
            reply.send({ error: "Forbidden", statusCode: 403 });
            return;
          }
        }
      }
    });

    // Auth enforcement: redirect unauthenticated browser navigations to the
    // login page. Health, auth, and asset endpoints remain public.
    app.addHook("onRequest", async (request, reply) => {
      const pathname = new URL(request.url, "http://bff.invalid").pathname;
      if (
        pathname.startsWith("/auth/") ||
        pathname.startsWith("/health/") ||
        pathname.startsWith("/assets/")
      ) {
        return;
      }
      if (request.method === "GET" && isApplicationRoute(pathname)) {
        const accessToken = request.session.get("accessToken");
        if (!accessToken) {
          reply.redirect("/auth/login");
          return;
        }
      }
    });
  }

  await app.register(fastifyStatic, {
    root: path.join(config.staticRoot, "assets"),
    prefix: "/assets/",
    decorateReply: false,
    immutable: true,
    maxAge: "1y",
    index: false,
    redirect: false,
  });

  app.get("/health/live", async (_request, reply) => {
    reply.header("Cache-Control", "no-store");
    return { status: "ok" };
  });

  app.get("/health/ready", async (_request, reply) => {
    reply.header("Cache-Control", "no-store");
    return { status: "ready" };
  });

  const sendApplication = (
    _request: unknown,
    reply: {
      header(name: string, value: string): unknown;
      type(value: string): { send(payload: string): unknown };
    },
  ) => {
    reply.header("Cache-Control", "no-store");
    return reply.type("text/html; charset=utf-8").send(indexDocument);
  };

  // Signals the browser to re-authenticate at the IdP. The SPA's API transport
  // turns this into a full-page redirect to /auth/login rather than surfacing a
  // generic error for an expired or revoked session.
  const respondReauth = (reply: {
    code(statusCode: number): unknown;
    header(name: string, value: string): unknown;
  }) => {
    reply.header("WWW-Authenticate", 'Bearer error="invalid_token"');
    reply.header("Cache-Control", "no-store");
    reply.code(401);
    return {
      error: "reauth_required",
      login_url: "/auth/login",
      statusCode: 401,
    };
  };

  // Same-origin browser telemetry ingest. The browser exporter posts OTLP/HTTP
  // JSON here; the BFF validates it and relays it to the configured collector,
  // keeping the collector origin out of the browser and reusing the session and
  // CSRF controls the other state-changing routes rely on (WEB-TRACE-02). The
  // global 1 MiB body limit bounds the payload.
  app.post("/telemetry/v1/traces", async (request, reply) => {
    reply.header("Cache-Control", "no-store");
    if (config.oidcIssuer && !request.session.get("accessToken")) {
      reply.code(401);
      return { error: "Unauthorized", statusCode: 401 };
    }
    const result = await tracing.ingestTraces(request.body);
    if (result === "rejected") {
      reply.code(400);
      return { error: "Bad Request", statusCode: 400 };
    }
    // "accepted" and "unavailable" are both success from the browser's view:
    // telemetry is best-effort and must never surface a collector outage.
    reply.code(202);
    return { status: "accepted" };
  });

  app.all("/api/*", async (request, reply) => {
    // Start one BFF server span per proxied request. It continues a valid
    // inbound W3C context and yields the validated upstream context to set on
    // the API request, so browser, BFF, and API join one trace (WEB-TRACE-04,
    // WEB-TRACE-05). A malformed inbound value is never forwarded: only the
    // span-derived context reaches upstream.
    const span = tracing.startProxySpan({
      correlationId: request.correlationId,
      method: request.method,
      routeTemplate: request.routeOptions.url ?? "/api/*",
      traceparent: singleHeaderValue(request.headers.traceparent),
      tracestate: singleHeaderValue(request.headers.tracestate),
    });
    // The span ends in the finally block with the outcome recorded here, so a
    // tracing failure never changes the proxy result (WEB-TRACE-09).
    let spanOutcome: ProxyOutcome = "server_error";
    let spanStatusCode = 500;
    const recordOutcome = (statusCode: number) => {
      spanOutcome = outcomeForStatus(statusCode);
      spanStatusCode = statusCode;
    };

    try {
      const incoming = new URL(request.url, "http://bff.invalid");
      const target = new URL(
        `${incoming.pathname}${incoming.search}`,
        config.apiOrigin,
      );
      const headers = new Headers();
      for (const header of forwardedRequestHeaders) {
        const value = request.headers[header];
        if (typeof value === "string") {
          headers.set(header, value);
        }
      }
      headers.set(correlationHeader, request.correlationId);
      const upstreamTrace = span.upstream();
      if (upstreamTrace) {
        headers.set("traceparent", upstreamTrace.traceparent);
        if (upstreamTrace.tracestate) {
          headers.set("tracestate", upstreamTrace.tracestate);
        }
      }

      const refreshToken = config.oidcIssuer
        ? request.tokenSession.get("refreshToken")
        : undefined;
      let refreshed = false;

      // Ensure a valid access token before forwarding (proactive refresh).
      if (config.oidcIssuer) {
        let accessToken = request.session.get("accessToken");
        const expiresAt = request.session.get("expiresAt");
        if (!accessToken || tokenExpired(expiresAt)) {
          if (!refreshToken || !app.refreshAccessToken) {
            clearSession(request);
            recordOutcome(401);
            return respondReauth(reply);
          }
          try {
            const tokens = await app.refreshAccessToken(refreshToken);
            persistTokenSet(request, tokens);
            accessToken = tokens.accessToken;
            refreshed = true;
          } catch {
            clearSession(request);
            recordOutcome(401);
            return respondReauth(reply);
          }
        }
        headers.set("authorization", `Bearer ${accessToken}`);
      }

      // Perform the upstream request with its own timeout and downstream-abort
      // wiring, so it can be retried once after a reactive refresh.
      const runUpstream = async (): Promise<Response> => {
        const controller = new AbortController();
        const timeoutReason = new Error("Upstream API request timed out");
        const timeout = setTimeout(() => {
          controller.abort(timeoutReason);
        }, config.apiTimeoutMs);
        const abortDownstream = () => {
          controller.abort();
        };
        request.raw.once("aborted", abortDownstream);
        try {
          return await fetch(target, {
            body: proxyBody(request.method, request.body),
            headers,
            method: request.method,
            redirect: "manual",
            signal: controller.signal,
          });
        } catch (error) {
          if (error === timeoutReason) {
            const timeout504 = new Error("Upstream API request timed out");
            timeout504.name = "UpstreamTimeout";
            throw timeout504;
          }
          throw error;
        } finally {
          clearTimeout(timeout);
          request.raw.off("aborted", abortDownstream);
        }
      };

      try {
        let upstream = await runUpstream();

        // Reactive refresh: a token we believed valid was rejected upstream
        // (clock skew, revocation, or key rotation). Refresh once and retry.
        if (
          config.oidcIssuer &&
          upstream.status === 401 &&
          !refreshed &&
          refreshToken &&
          app.refreshAccessToken
        ) {
          try {
            const tokens = await app.refreshAccessToken(refreshToken);
            persistTokenSet(request, tokens);
            headers.set("authorization", `Bearer ${tokens.accessToken}`);
            upstream = await runUpstream();
          } catch {
            // Fall through to the re-authentication response below.
          }
        }

        if (config.oidcIssuer && upstream.status === 401) {
          clearSession(request);
          recordOutcome(401);
          return respondReauth(reply);
        }

        for (const header of forwardedResponseHeaders) {
          const value = upstream.headers.get(header);
          if (value !== null) {
            reply.header(header, value);
          }
        }
        reply.code(upstream.status);
        recordOutcome(upstream.status);
        if (request.method === "HEAD" || upstream.status === 204) {
          return await reply.send();
        }
        return await reply.send(Buffer.from(await upstream.arrayBuffer()));
      } catch (error) {
        if (error instanceof Error && error.name === "UpstreamTimeout") {
          reply.code(504);
          spanOutcome = "timeout";
          spanStatusCode = 504;
          return {
            error: "Gateway Timeout",
            statusCode: 504,
          };
        }
        throw error;
      }
    } finally {
      span.end(spanOutcome, spanStatusCode);
    }
  });

  app.setNotFoundHandler(async (request, reply) => {
    if (
      request.method === "GET" &&
      isApplicationRoute(new URL(request.url, "http://localhost").pathname)
    ) {
      return sendApplication(request, reply);
    }
    reply.code(404);
    return { error: "Not Found", statusCode: 404 };
  });

  app.setErrorHandler(async (error, request, reply) => {
    request.log.error({ err: error }, "request failed");
    const errorStatus =
      typeof error === "object" &&
      error !== null &&
      "statusCode" in error &&
      typeof error.statusCode === "number"
        ? error.statusCode
        : 500;
    reply.code(errorStatus >= 400 && errorStatus < 500 ? errorStatus : 500);
    return {
      error:
        reply.statusCode < 500 ? "Request failed" : "Internal Server Error",
      statusCode: reply.statusCode,
    };
  });

  return app;
}
